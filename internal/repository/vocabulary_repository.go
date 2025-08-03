package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"language-assistant/internal/models"
	"language-assistant/internal/utils"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/sirupsen/logrus"
)

type vocabularyRepository struct {
	logger    *logrus.Entry
	dynamodb  utils.DynamoDbAPI
	tableName string
}

func NewVocabularyRepository(logger *logrus.Entry, dynamodb utils.DynamoDbAPI, tableName string) utils.VocabularyRepository {
	return &vocabularyRepository{
		logger:    logger,
		dynamodb:  dynamodb,
		tableName: tableName,
	}
}

func (r *vocabularyRepository) SaveWord(word, partOfSpeech, translation, sentence, userID string) error {
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	timestamp := now.Format(time.RFC3339)

	// 新的 key 結構：PK = userId#vocabulary, SK = date
	pk := fmt.Sprintf("%s#vocabulary", userID)
	
	// get user vocabulary of today
	result, err := r.dynamodb.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: pk},
			"sk": &types.AttributeValueMemberS{Value: today},
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

	_, err = r.dynamodb.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item: map[string]types.AttributeValue{
			"pk":        &types.AttributeValueMemberS{Value: pk},
			"sk":        &types.AttributeValueMemberS{Value: userVoca.Date},
			"userId":    &types.AttributeValueMemberS{Value: userVoca.UserID},
			"date":      &types.AttributeValueMemberS{Value: userVoca.Date},
			"words":     &types.AttributeValueMemberS{Value: string(wordsJSON)},
			"updatedAt": &types.AttributeValueMemberS{Value: userVoca.UpdatedAt},
		},
	})
	if err != nil {
		r.logger.WithError(err).Error("Failed to save user vocabulary to DynamoDB")
		return fmt.Errorf("failed to save user vocabulary: %w", err)
	}

	return nil
}

func (r *vocabularyRepository) GetUserVocabularyByDate(userID, date string) (*models.UserVocabulary, error) {
	pk := fmt.Sprintf("%s#vocabulary", userID)
	
	result, err := r.dynamodb.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: pk},
			"sk": &types.AttributeValueMemberS{Value: date},
		},
	})

	if err != nil {
		r.logger.WithError(err).Error("Failed to get user vocabulary from DynamoDB")
		return nil, fmt.Errorf("failed to get user vocabulary: %w", err)
	}

	if result.Item == nil {
		// No vocabulary found for this date
		return nil, nil
	}

	var userVoca models.UserVocabulary
	userVoca.UserID = userID
	userVoca.Date = date

	// Extract updatedAt
	if attr, ok := result.Item["updatedAt"].(*types.AttributeValueMemberS); ok {
		userVoca.UpdatedAt = attr.Value
	}

	// Extract and parse words
	if attr, ok := result.Item["words"].(*types.AttributeValueMemberS); ok {
		if err := json.Unmarshal([]byte(attr.Value), &userVoca.Words); err != nil {
			return nil, fmt.Errorf("failed to unmarshal words: %w", err)
		}
	} else {
		userVoca.Words = []models.WordRecord{}
	}

	return &userVoca, nil
}

func (r *vocabularyRepository) GetAllUserVocabularies(userID string) ([]models.UserVocabulary, error) {
	pk := fmt.Sprintf("%s#vocabulary", userID)
	
	result, err := r.dynamodb.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
		ScanIndexForward: aws.Bool(false), // 最新的日期在前
	})

	if err != nil {
		r.logger.WithError(err).Error("Failed to query user vocabularies from DynamoDB")
		return nil, fmt.Errorf("failed to query user vocabularies: %w", err)
	}

	if result.Items == nil {
		return []models.UserVocabulary{}, nil
	}

	var userVocabularies []models.UserVocabulary
	for _, item := range result.Items {
		var userVoca models.UserVocabulary
		userVoca.UserID = userID

		// Extract date from SK
		if attr, ok := item["sk"].(*types.AttributeValueMemberS); ok {
			userVoca.Date = attr.Value
		}

		// Extract updatedAt
		if attr, ok := item["updatedAt"].(*types.AttributeValueMemberS); ok {
			userVoca.UpdatedAt = attr.Value
		}

		// Extract and parse words
		if attr, ok := item["words"].(*types.AttributeValueMemberS); ok {
			if err := json.Unmarshal([]byte(attr.Value), &userVoca.Words); err != nil {
				r.logger.WithError(err).Error("Failed to unmarshal words field")
				continue
			}
		} else {
			userVoca.Words = []models.WordRecord{}
		}

		userVocabularies = append(userVocabularies, userVoca)
	}

	r.logger.WithFields(logrus.Fields{
		"userId": userID,
		"count":  len(userVocabularies),
	}).Info("Successfully retrieved user vocabularies")

	return userVocabularies, nil
}