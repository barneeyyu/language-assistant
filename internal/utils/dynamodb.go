package utils

import (
	"context"
	"language-assistant/internal/models"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// DynamoDbAPI defines the DynamoDB operations needed by our application
type DynamoDbAPI interface {
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
}

// VocabularyRepository defines vocabulary-related database operations
type VocabularyRepository interface {
	SaveWord(word, partOfSpeech, translation, sentence, userID string) error
}

// ReminderRepository defines reminder-related database operations
type ReminderRepository interface {
	GetUserVocabulariesByDate(date string) ([]models.UserVocabulary, error)
}