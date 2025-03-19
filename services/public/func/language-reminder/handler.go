package main

import (
	"context"
	"encoding/json"
	"fmt"
	"language-assistant/models"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/line/line-bot-sdk-go/v7/linebot"
	"github.com/sirupsen/logrus"
)

type Handler struct {
	logger  *logrus.Entry
	envVars *EnvVars
}

type ReminderEvent struct {
	Date string `json:"date"`
}

type DynamoDbAPI interface {
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

func NewHandler(logger *logrus.Entry, envVars *EnvVars) (*Handler, error) {
	return &Handler{
		logger:  logger,
		envVars: envVars,
	}, nil
}

func (h *Handler) EventHandler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if req.Body != "" {
		var event ReminderEvent
		if err := json.Unmarshal([]byte(req.Body), &event); err != nil {
			h.logger.WithError(err).Error("Failed to decode reminder event")
			return events.APIGatewayProxyResponse{
				StatusCode: 400,
				Body:       "Bad request",
			}, nil
		}
		h.logger.Info("Getting the request for date: ", event.Date)
	}

	date := time.Now().Format("2006-01-02")
	userVoca, err := h.getWord(date)
	if err != nil {
		h.logger.WithError(err).Error("Failed to get word")
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Body:       "Internal server error",
		}, nil
	}

	for index, dailyUserData := range userVoca {
		h.logger.Infof("Currently handle %d user: %s", index, dailyUserData.UserID)
		message := linebot.NewTextMessage(models.FormatWordRecords(dailyUserData.Words)).
			WithSender(&linebot.Sender{
				Name: "Reminder Bot",
			})
		if _, err := h.envVars.botClient.PushMessage(dailyUserData.UserID, message).Do(); err != nil {
			h.logger.WithError(err).Error("Failed to send reminder message")
			return events.APIGatewayProxyResponse{
				StatusCode: 500,
				Body:       "Internal server error",
			}, nil
		}
	}
	h.logger.Info("Successfully sent reminder message")
	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       "Reminder sent",
	}, nil
}

func (h *Handler) getWord(date string) ([]models.UserVocabulary, error) {
	result, err := h.envVars.dynamodbClient.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(h.envVars.vocabularyTableName),
		KeyConditionExpression: aws.String("#date = :dateVal"), // Use #date as an alias to avoid using the reserved keyword "date"
		ExpressionAttributeNames: map[string]string{
			"#date": "date", // Define #date to reference the "date" column in DynamoDB
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":dateVal": &types.AttributeValueMemberS{Value: date},
		},
	})

	if err != nil {
		h.logger.WithError(err).Error("Failed to get word from DynamoDB")
		return nil, fmt.Errorf("failed to get word: %w", err)
	}

	if result.Items == nil {
		h.logger.Warn("No word found for the given date")
		return nil, nil
	}

	// Parse DynamoDB items into UserVocabulary structs
	var userVocabularies []models.UserVocabulary
	for _, item := range result.Items {
		var userVoca models.UserVocabulary

		// Extract `userId`
		if attr, ok := item["userId"].(*types.AttributeValueMemberS); ok {
			userVoca.UserID = attr.Value
		}

		// Extract `date`
		if attr, ok := item["date"].(*types.AttributeValueMemberS); ok {
			userVoca.Date = attr.Value
		}

		// Extract `updatedAt`
		if attr, ok := item["updatedAt"].(*types.AttributeValueMemberS); ok {
			userVoca.UpdatedAt = attr.Value
		}

		// Extract and parse `words` (which is a JSON-encoded string in DynamoDB)
		if attr, ok := item["words"].(*types.AttributeValueMemberS); ok {
			var words []models.WordRecord
			if err := json.Unmarshal([]byte(attr.Value), &words); err != nil {
				h.logger.WithError(err).Error("Failed to unmarshal words field")
				return nil, fmt.Errorf("failed to parse words field: %w", err)
			}
			userVoca.Words = words
		}

		userVocabularies = append(userVocabularies, userVoca)
	}

	h.logger.Info("Successfully retrieved user vocabularies: ", userVocabularies)
	return userVocabularies, nil
}
