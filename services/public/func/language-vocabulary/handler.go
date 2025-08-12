package main

import (
	"encoding/json"
	"fmt"
	"language-assistant/internal/utils"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/sirupsen/logrus"
)

type Handler struct {
	logger          *logrus.Entry
	envVars         *EnvVars
	openaiClient    utils.OpenaiAPI
	linebotClient   utils.LinebotAPI
	userConfigRepo  utils.UserConfigRepository
	bloomFilterRepo utils.BloomFilterRepository
}

func NewHandler(logger *logrus.Entry, envVars *EnvVars, openaiClient utils.OpenaiAPI, linebotClient utils.LinebotAPI, userConfigRepo utils.UserConfigRepository, bloomFilterRepo utils.BloomFilterRepository) (*Handler, error) {
	return &Handler{
		logger:          logger,
		envVars:         envVars,
		openaiClient:    openaiClient,
		linebotClient:   linebotClient,
		userConfigRepo:  userConfigRepo,
		bloomFilterRepo: bloomFilterRepo,
	}, nil
}

type WordPushRequest struct {
	UserID string `json:"userId"`
}

type WordPushResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// HandleWordPush 處理 Lambda invoke 的請求
func (h *Handler) HandleWordPush(request map[string]string) (map[string]interface{}, error) {
	h.logger.Info("Received direct word push request")

	userID := request["userId"]
	if userID == "" {
		h.logger.Error("User ID is required")
		return map[string]interface{}{
			"status":  "error",
			"message": "User ID is required",
		}, nil
	}

	// Get user configuration
	userConfig, err := h.userConfigRepo.GetUserConfig(userID)
	if err != nil {
		h.logger.WithError(err).Error("Failed to get user config")
		return map[string]interface{}{
			"status":  "error",
			"message": "Failed to get user configuration",
		}, nil
	}

	if userConfig == nil {
		h.logger.Error("User config not found")
		return map[string]interface{}{
			"status":  "error",
			"message": "User configuration not found",
		}, nil
	}

	// Generate words based on user configuration with Bloom Filter
	words, err := h.generateWordsWithBloomFilter(userID, userConfig.Course, userConfig.DailyWords, userConfig.Level)
	if err != nil {
		h.logger.WithError(err).Error("Failed to generate words")
		return map[string]interface{}{
			"status":  "error",
			"message": "Failed to generate words",
		}, nil
	}

	// Send words to user via LINE Bot
	err = h.sendWordsToUser(userID, words, userConfig.Course)
	if err != nil {
		h.logger.WithError(err).Error("Failed to send words to user")
		return map[string]interface{}{
			"status":  "error",
			"message": "Failed to send words to user",
		}, nil
	}

	// Add sent words to Bloom Filter
	err = h.bloomFilterRepo.AddWordsToBloomFilter(userID, userConfig.Course, words)
	if err != nil {
		h.logger.WithError(err).Warn("Failed to add words to bloom filter") // Non-critical error
	}

	h.logger.WithFields(logrus.Fields{
		"userId": userID,
		"course": userConfig.Course,
		"count":  len(words),
	}).Info("Successfully pushed words to user")

	return map[string]interface{}{
		"status":  "success",
		"message": "Words sent successfully",
		"data": map[string]interface{}{
			"userId":    userID,
			"course":    userConfig.Course,
			"wordCount": len(words),
		},
	}, nil
}

// func (h *Handler) HandleWordPush(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
// 	h.logger.Info("Received word push request")

// 	// Parse request body
// 	var req WordPushRequest
// 	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
// 		h.logger.WithError(err).Error("Failed to parse request body")
// 		return h.errorResponse(400, "Invalid request body"), nil
// 	}

// 	if req.UserID == "" {
// 		h.logger.Error("User ID is required")
// 		return h.errorResponse(400, "User ID is required"), nil
// 	}

// 	// Get user configuration
// 	userConfig, err := h.userConfigRepo.GetUserConfig(req.UserID)
// 	if err != nil {
// 		h.logger.WithError(err).Error("Failed to get user config")
// 		return h.errorResponse(500, "Failed to get user configuration"), nil
// 	}

// 	if userConfig == nil {
// 		h.logger.Error("User config not found")
// 		return h.errorResponse(404, "User configuration not found"), nil
// 	}

// 	// Generate words based on user configuration with Bloom Filter
// 	words, err := h.generateWordsWithBloomFilter(req.UserID, userConfig.Course, userConfig.DailyWords, userConfig.Level)
// 	if err != nil {
// 		h.logger.WithError(err).Error("Failed to generate words")
// 		return h.errorResponse(500, "Failed to generate words"), nil
// 	}

// 	// Send words to user via LINE Bot
// 	err = h.sendWordsToUser(req.UserID, words, userConfig.Course)
// 	if err != nil {
// 		h.logger.WithError(err).Error("Failed to send words to user")
// 		return h.errorResponse(500, "Failed to send words to user"), nil
// 	}

// 	// Add sent words to Bloom Filter
// 	err = h.bloomFilterRepo.AddWordsToBloomFilter(req.UserID, userConfig.Course, words)
// 	if err != nil {
// 		h.logger.WithError(err).Warn("Failed to add words to bloom filter") // Non-critical error
// 	}

// 	h.logger.WithFields(logrus.Fields{
// 		"userId": req.UserID,
// 		"course": userConfig.Course,
// 		"count":  len(words),
// 	}).Info("Successfully pushed words to user")

// 	response := WordPushResponse{
// 		Status:  "success",
// 		Message: "Words sent successfully",
// 		Data: map[string]interface{}{
// 			"userId":    req.UserID,
// 			"course":    userConfig.Course,
// 			"wordCount": len(words),
// 		},
// 	}

// 	return h.successResponse(response), nil
// }

func (h *Handler) errorResponse(statusCode int, message string) events.APIGatewayProxyResponse {
	response := WordPushResponse{
		Status:  "error",
		Message: message,
	}

	body, _ := json.Marshal(response)
	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}
}

func (h *Handler) successResponse(data WordPushResponse) events.APIGatewayProxyResponse {
	body, _ := json.Marshal(data)
	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}
}

func (h *Handler) generateWords(course string, wordCount int, level int) ([]utils.Word, error) {
	wordResponse, err := h.openaiClient.GenerateWord(course, wordCount, level)
	if err != nil {
		return nil, fmt.Errorf("failed to generate words: %w", err)
	}

	return wordResponse.Words, nil
}

func (h *Handler) generateWordsWithBloomFilter(userID, course string, wordCount int, level int) ([]utils.Word, error) {
	// Generate more words than needed to account for filtering
	generateCount := wordCount * 3 // Generate 3x to account for duplicates
	maxAttempts := 5

	var finalWords []utils.Word

	for attempt := 1; attempt <= maxAttempts && len(finalWords) < wordCount; attempt++ {
		h.logger.Infof("Attempt %d to generate %d words for user %s", attempt, generateCount, userID)

		// Generate words using OpenAI
		words, err := h.generateWords(course, generateCount, level)
		if err != nil {
			return nil, fmt.Errorf("failed to generate words on attempt %d: %w", attempt, err)
		}

		h.logger.Infof("OpenAI returned %d words", len(words))

		// Filter out words already in Bloom Filter
		newWords, err := h.bloomFilterRepo.FilterWords(userID, course, words)
		if err != nil {
			return nil, fmt.Errorf("failed to filter words: %w", err)
		}

		// Add new words to our final list
		for _, word := range newWords {
			if len(finalWords) < wordCount {
				finalWords = append(finalWords, word)
			} else {
				break
			}
		}

		h.logger.Infof("Generated %d words, filtered to %d new words, total collected: %d/%d",
			len(words), len(newWords), len(finalWords), wordCount)

		// If we have enough words, break early
		if len(finalWords) >= wordCount {
			break
		}

		// If we don't have enough words yet, increase generation count for next attempt
		generateCount = wordCount * 5 // Increase more aggressively
	}

	if len(finalWords) == 0 {
		return nil, fmt.Errorf("failed to generate any new words after %d attempts", maxAttempts)
	}

	// 確保返回確切的數量，如果生成的超過需求則截取
	if len(finalWords) > wordCount {
		finalWords = finalWords[:wordCount]
	}

	h.logger.Infof("Successfully generated %d unique words for user %s", len(finalWords), userID)
	return finalWords, nil
}

func (h *Handler) sendWordsToUser(userID string, words []utils.Word, course string) error {
	if len(words) == 0 {
		return fmt.Errorf("no words to send")
	}

	var messages []string
	messages = append(messages, fmt.Sprintf("📚 今日%s單字推播 (%d個)", course, len(words)))
	messages = append(messages, "")

	for i, word := range words {
		wordText := fmt.Sprintf("%d. 【%s】(%s)\n意思：%s\n例句：%s\n中文：%s",
			i+1,
			word.Word,
			word.PartOfSpeech,
			word.Meaning,
			word.Example.En,
			word.Example.Zh,
		)

		if len(word.Synonyms) > 0 {
			wordText += fmt.Sprintf("\n同義詞：%s", strings.Join(word.Synonyms, ", "))
		}

		if len(word.Antonyms) > 0 {
			wordText += fmt.Sprintf("\n反義詞：%s", strings.Join(word.Antonyms, ", "))
		}

		messages = append(messages, wordText)
		messages = append(messages, "")
	}

	finalMessage := strings.Join(messages, "\n")

	err := h.linebotClient.PushMessage(userID, finalMessage)
	if err != nil {
		return fmt.Errorf("failed to push message to user: %w", err)
	}

	return nil
}
