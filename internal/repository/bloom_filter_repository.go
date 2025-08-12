package repository

import (
	"context"
	"fmt"
	"language-assistant/internal/models"
	"language-assistant/internal/utils"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/sirupsen/logrus"
)

type BloomFilterRepository struct {
	logger    *logrus.Entry
	client    utils.DynamoDbAPI
	tableName string
}

func NewBloomFilterRepository(logger *logrus.Entry, client utils.DynamoDbAPI, tableName string) utils.BloomFilterRepository {
	return &BloomFilterRepository{
		logger:    logger,
		client:    client,
		tableName: tableName,
	}
}

func (r *BloomFilterRepository) GetBloomFilter(userID, course string) (*models.BloomFilter, error) {
	input := &dynamodb.GetItemInput{
		TableName: &r.tableName,
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: userID + "#bloomFilter"},
			"sk": &types.AttributeValueMemberS{Value: course}, // "toeic" or "ielts"
		},
	}

	result, err := r.client.GetItem(context.Background(), input)
	if err != nil {
		r.logger.WithError(err).Error("Failed to get bloom filter from DynamoDB")
		return nil, fmt.Errorf("failed to get bloom filter: %w", err)
	}

	if result.Item == nil {
		// Return a new Bloom Filter if one doesn't exist
		r.logger.Infof("No existing bloom filter found for user %s course %s, creating new one", userID, course)
		return models.NewBloomFilter(userID, 10000), nil
	}

	var bloomFilter models.BloomFilter
	err = attributevalue.UnmarshalMap(result.Item, &bloomFilter)
	if err != nil {
		r.logger.WithError(err).Error("Failed to unmarshal bloom filter")
		return nil, fmt.Errorf("failed to unmarshal bloom filter: %w", err)
	}

	return &bloomFilter, nil
}

func (r *BloomFilterRepository) SaveBloomFilter(filter *models.BloomFilter, course string) error {
	filter.UpdatedAt = time.Now().Format(time.RFC3339)

	item, err := attributevalue.MarshalMap(filter)
	if err != nil {
		r.logger.WithError(err).Error("Failed to marshal bloom filter")
		return fmt.Errorf("failed to marshal bloom filter: %w", err)
	}

	// Set the partition key and sort key
	item["pk"] = &types.AttributeValueMemberS{Value: filter.UserID + "#bloomFilter"}
	item["sk"] = &types.AttributeValueMemberS{Value: course} // "toeic" or "ielts"

	input := &dynamodb.PutItemInput{
		TableName: &r.tableName,
		Item:      item,
	}

	_, err = r.client.PutItem(context.Background(), input)
	if err != nil {
		r.logger.WithError(err).Error("Failed to save bloom filter to DynamoDB")
		return fmt.Errorf("failed to save bloom filter: %w", err)
	}

	r.logger.Infof("Successfully saved bloom filter for user %s", filter.UserID)
	return nil
}

func (r *BloomFilterRepository) AddWordToBloomFilter(userID, word, course string) error {
	// Get existing bloom filter
	filter, err := r.GetBloomFilter(userID, course)
	if err != nil {
		return fmt.Errorf("failed to get bloom filter: %w", err)
	}

	// Add word to bloom filter
	filter.Add(word)

	// Save updated bloom filter
	err = r.SaveBloomFilter(filter, course)
	if err != nil {
		return fmt.Errorf("failed to save updated bloom filter: %w", err)
	}

	r.logger.Infof("Added word '%s' to bloom filter for user %s course %s", word, userID, course)
	return nil
}

// FilterWords removes words that are already in the bloom filter
func (r *BloomFilterRepository) FilterWords(userID, course string, words []utils.Word) ([]utils.Word, error) {
	filter, err := r.GetBloomFilter(userID, course)
	if err != nil {
		return nil, fmt.Errorf("failed to get bloom filter: %w", err)
	}

	var filteredWords []utils.Word
	for _, word := range words {
		if !filter.Contains(word.Word) {
			filteredWords = append(filteredWords, word)
		} else {
			r.logger.Debugf("Word '%s' already exists in bloom filter for user %s course %s, skipping", word.Word, userID, course)
		}
	}

	r.logger.Infof("Filtered %d words for user %s course %s, %d words remaining", 
		len(words)-len(filteredWords), userID, course, len(filteredWords))

	return filteredWords, nil
}

// AddWordsToBloomFilter adds multiple words to the bloom filter
func (r *BloomFilterRepository) AddWordsToBloomFilter(userID, course string, words []utils.Word) error {
	filter, err := r.GetBloomFilter(userID, course)
	if err != nil {
		return fmt.Errorf("failed to get bloom filter: %w", err)
	}

	r.logger.Infof("Before adding words: BitArray size=%d, first 10 bytes: %v", len(filter.BitArray), filter.BitArray[:10])

	for i, word := range words {
		r.logger.Debugf("Adding word %d: %s", i+1, word.Word)
		filter.Add(word.Word)
	}

	r.logger.Infof("After adding words: first 10 bytes: %v", filter.BitArray[:10])

	err = r.SaveBloomFilter(filter, course)
	if err != nil {
		return fmt.Errorf("failed to save updated bloom filter: %w", err)
	}

	r.logger.Infof("Added %d words to bloom filter for user %s course %s", len(words), userID, course)
	return nil
}