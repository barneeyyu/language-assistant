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
	GetUserVocabularyByDate(userID, date string) (*models.UserVocabulary, error)
	GetAllUserVocabularies(userID string) ([]models.UserVocabulary, error)
}

// ReminderRepository defines reminder-related database operations
type ReminderRepository interface {
	GetUserVocabulariesByDate(date string) ([]models.UserVocabulary, error)
}

// UserConfigRepository defines user configuration database operations
type UserConfigRepository interface {
	SaveUserConfig(userID, course string, level int, dailyWords int, pushTime, timezone string) error
	GetUserConfig(userID string) (*models.UserConfig, error)
	GetUsersByCourse(course string) ([]models.UserConfig, error)
}

// BloomFilterRepository defines Bloom Filter related database operations
type BloomFilterRepository interface {
	GetBloomFilter(userID, course string) (*models.BloomFilter, error)
	SaveBloomFilter(filter *models.BloomFilter, course string) error
	AddWordToBloomFilter(userID, word, course string) error
	FilterWords(userID, course string, words []Word) ([]Word, error)
	AddWordsToBloomFilter(userID, course string, words []Word) error
}