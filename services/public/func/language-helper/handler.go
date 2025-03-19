package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"language-assistant/utils"
	"net/http"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/line/line-bot-sdk-go/v7/linebot"
	"github.com/sirupsen/logrus"

	"language-assistant/models"
)

type DynamoDbAPI interface {
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
}

type Handler struct {
	logger         *logrus.Entry
	envVars        *EnvVars
	linebotClient  utils.LinebotAPI
	openaiClient   utils.OpenaiAPI
	dynamodbClient DynamoDbAPI
}

func NewHandler(logger *logrus.Entry, envVars *EnvVars, linebotClient utils.LinebotAPI, openaiClient utils.OpenaiAPI, dynamodbClient DynamoDbAPI) (*Handler, error) {
	return &Handler{
		logger:         logger,
		envVars:        envVars,
		linebotClient:  linebotClient,
		openaiClient:   openaiClient,
		dynamodbClient: dynamodbClient,
	}, nil
}

func (h *Handler) EventHandler(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	h.logger.Info("Received request: ", request)

	messageEvents, err := h.RequestParser(request)
	if err != nil {
		h.logger.Error("Failed to parse request: ", err)
		return events.APIGatewayProxyResponse{
			StatusCode: 400,
			Body:       "Bad Request",
		}, nil
	}

	// Process each message event
	for _, event := range messageEvents {
		h.logger.WithFields(logrus.Fields{
			"event_type": event.Type,
			"user_id":    event.Source.UserID,
			"room_id":    event.Source.RoomID,
			"group_id":   event.Source.GroupID,
		}).Info("event handling")
		// Check if it's a message event
		if event.Type == linebot.EventTypeMessage {
			switch message := event.Message.(type) {
			case *linebot.TextMessage:
				translationResponse, err := h.openaiClient.Translate(message.Text)
				if err != nil {
					h.logger.WithError(err).Error("Failed to transfer valid schedule")
					return events.APIGatewayProxyResponse{
						Body:       err.Error(),
						StatusCode: 500,
					}, nil
				}
				h.logger.Info("Translation response: ", translationResponse)

				for _, translation := range translationResponse.Translations {
					if err := h.saveWord(translation.Word, translation.PartOfSpeech, translation.Meaning, translation.Example.En, event.Source.UserID); err != nil {
						h.logger.Error("Failed to save word: ", err)
						continue
					}
				}
				// Reply with the same message
				if err := h.linebotClient.ReplyMessage(event.ReplyToken, translationResponse.String()); err != nil {
					h.logger.Error("Failed to reply message: ", err)
					continue
				}
			}
		}
	}

	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       "OK",
	}, nil
}

func (h *Handler) saveWord(word, partOfSpeech, translation, sentence, userID string) error {
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	timestamp := now.Format(time.RFC3339)

	// get user vocabulary of today
	result, err := h.dynamodbClient.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(h.envVars.vocabularyTableName),
		Key: map[string]types.AttributeValue{
			"date":   &types.AttributeValueMemberS{Value: today},
			"userId": &types.AttributeValueMemberS{Value: userID},
		},
	})

	// make sure that search DB without error
	if err != nil {
		return fmt.Errorf("failed to get user vocabulary from DynamoDB: %w", err)
	}

	var userVoca models.UserVocabulary
	// if record not found, create new record
	if result.Item == nil {
		// create new user vocabulary
		userVoca = models.UserVocabulary{
			Date:      today,
			UserID:    userID,
			Words:     []models.WordRecord{},
			UpdatedAt: timestamp,
		}
	} else {
		// if record exists, update the record
		userVoca.Date = today
		userVoca.UserID = userID
		userVoca.UpdatedAt = timestamp

		// parse words from dynamodb
		if wordsAttr, ok := result.Item["words"].(*types.AttributeValueMemberS); ok && wordsAttr != nil {
			if err := json.Unmarshal([]byte(wordsAttr.Value), &userVoca.Words); err != nil {
				return fmt.Errorf("failed to unmarshal words: %w", err)
			}
		} else {
			userVoca.Words = []models.WordRecord{}
		}
	}

	// add new word to user vocabulary no matter it's already in the list or not
	userVoca.Words = append(userVoca.Words, models.WordRecord{
		Word:         word,
		PartOfSpeech: partOfSpeech,
		Translation:  translation,
		Sentence:     sentence,
		Timestamp:    timestamp,
	})
	userVoca.UpdatedAt = timestamp

	// save user vocabulary to dynamodb
	wordsJSON, err := json.Marshal(userVoca.Words)
	if err != nil {
		return errors.New("failed to marshal words")
	}

	_, err = h.dynamodbClient.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(h.envVars.vocabularyTableName),
		Item: map[string]types.AttributeValue{
			"date":      &types.AttributeValueMemberS{Value: userVoca.Date},
			"userId":    &types.AttributeValueMemberS{Value: userVoca.UserID},
			"words":     &types.AttributeValueMemberS{Value: string(wordsJSON)},
			"updatedAt": &types.AttributeValueMemberS{Value: userVoca.UpdatedAt},
		},
	})
	if err != nil {
		h.logger.WithError(err).Error("Failed to save user vocabulary to DynamoDB")
		return fmt.Errorf("failed to save user vocabulary: %w", err)
	}

	return err
}

func (h *Handler) RequestParser(request events.APIGatewayProxyRequest) ([]*linebot.Event, error) {
	var bodyJSON interface{}
	if err := json.Unmarshal([]byte(request.Body), &bodyJSON); err != nil {
		h.logger.WithError(err).Error("Failed to parse JSON")
		return nil, err
	} else {
		h.logger.WithFields(logrus.Fields{
			"webhook_body": bodyJSON,
		}).Info("Received LINE webhook")
	}

	// analyze request body
	reqBody := bytes.NewBufferString(request.Body)
	req, err := http.NewRequest(http.MethodPost, "", reqBody)
	if err != nil {
		return nil, err
	}

	// parse all headers
	req.Header = make(http.Header)
	for key, value := range request.Headers {
		req.Header.Set(key, value)
	}
	// Parse the webhook event
	messageEvents, err := h.linebotClient.ParseRequest(req)
	if err != nil {
		h.logger.Error("Failed to parse webhook request: ", err)
		return nil, err
	}

	return messageEvents, nil
}
