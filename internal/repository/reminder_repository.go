package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"language-assistant/internal/models"
	"language-assistant/internal/utils"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/sirupsen/logrus"
)

type reminderRepository struct {
	logger    *logrus.Entry
	dynamodb  utils.DynamoDbAPI
	tableName string
}

func NewReminderRepository(logger *logrus.Entry, dynamodb utils.DynamoDbAPI, tableName string) utils.ReminderRepository {
	return &reminderRepository{
		logger:    logger,
		dynamodb:  dynamodb,
		tableName: tableName,
	}
}

func (r *reminderRepository) GetUserVocabulariesByDate(date string) ([]models.UserVocabulary, error) {
	result, err := r.dynamodb.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("#date = :dateVal"), // Use #date as an alias to avoid using the reserved keyword "date"
		ExpressionAttributeNames: map[string]string{
			"#date": "date", // Define #date to reference the "date" column in DynamoDB
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":dateVal": &types.AttributeValueMemberS{Value: date},
		},
	})

	if err != nil {
		r.logger.WithError(err).Error("Failed to get word from DynamoDB")
		return nil, fmt.Errorf("failed to get word: %w", err)
	}

	if result.Items == nil {
		r.logger.Warn("No word found for the given date")
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
				r.logger.WithError(err).Error("Failed to unmarshal words field")
				return nil, fmt.Errorf("failed to parse words field: %w", err)
			}
			userVoca.Words = words
		}

		userVocabularies = append(userVocabularies, userVoca)
	}

	r.logger.Info("Successfully retrieved user vocabularies: ", userVocabularies)
	return userVocabularies, nil
}