package repository

import (
	"context"
	"fmt"
	"language-assistant/internal/models"
	"language-assistant/internal/utils"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/sirupsen/logrus"
)

type userConfigRepository struct {
	logger    *logrus.Entry
	dynamodb  utils.DynamoDbAPI
	tableName string
}

func NewUserConfigRepository(logger *logrus.Entry, dynamodb utils.DynamoDbAPI, tableName string) utils.UserConfigRepository {
	return &userConfigRepository{
		logger:    logger,
		dynamodb:  dynamodb,
		tableName: tableName,
	}
}

func (r *userConfigRepository) SaveUserConfig(userID, displayName, course string, level int, dailyWords int, pushTime, timezone string) error {
	timestamp := time.Now().UTC().Format(time.RFC3339)

	item := map[string]types.AttributeValue{
		"userId":    &types.AttributeValueMemberS{Value: userID},
		"updatedAt": &types.AttributeValueMemberS{Value: timestamp},
	}

	// 只在有值時才設定欄位
	if displayName != "" {
		item["displayName"] = &types.AttributeValueMemberS{Value: displayName}
	}
	if course != "" {
		item["course"] = &types.AttributeValueMemberS{Value: course}
	}
	if level != 0 {
		item["level"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%d", level)}
	}
	if dailyWords != 0 {
		item["dailyWords"] = &types.AttributeValueMemberS{Value: fmt.Sprintf("%d", dailyWords)}
	}
	if pushTime != "" {
		item["pushTime"] = &types.AttributeValueMemberS{Value: pushTime}
	}
	if timezone != "" {
		item["timezone"] = &types.AttributeValueMemberS{Value: timezone}
	}

	_, err := r.dynamodb.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})

	if err != nil {
		r.logger.WithError(err).Error("Failed to save user config to DynamoDB")
		return fmt.Errorf("failed to save user config: %w", err)
	}

	r.logger.WithFields(logrus.Fields{
		"userId":     userID,
		"course":     course,
		"level":      level,
		"dailyWords": dailyWords,
		"pushTime":   pushTime,
		"timezone":   timezone,
	}).Info("Successfully saved user config")

	return nil
}

func (r *userConfigRepository) GetUserConfig(userID string) (*models.UserConfig, error) {
	result, err := r.dynamodb.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"userId": &types.AttributeValueMemberS{Value: userID},
		},
	})

	if err != nil {
		r.logger.WithError(err).Error("Failed to get user config from DynamoDB")
		return nil, fmt.Errorf("failed to get user config: %w", err)
	}

	if result.Item == nil {
		// User config not found
		return nil, nil
	}

	var userConfig models.UserConfig
	userConfig.UserID = userID

	// Extract displayName
	if attr, ok := result.Item["displayName"].(*types.AttributeValueMemberS); ok {
		userConfig.DisplayName = attr.Value
	}

	// Extract course
	if attr, ok := result.Item["course"].(*types.AttributeValueMemberS); ok {
		userConfig.Course = attr.Value
	}

	// Extract level
	if attr, ok := result.Item["level"].(*types.AttributeValueMemberS); ok {
		level, err := strconv.Atoi(attr.Value)
		if err == nil {
			userConfig.Level = level
		}
	}

	// Extract dailyWords
	if attr, ok := result.Item["dailyWords"].(*types.AttributeValueMemberS); ok {
		dailyWords, err := strconv.Atoi(attr.Value)
		if err == nil {
			userConfig.DailyWords = dailyWords
		}
	} else {
		userConfig.DailyWords = 10 // 預設值
	}

	// Extract pushTime
	if attr, ok := result.Item["pushTime"].(*types.AttributeValueMemberS); ok {
		userConfig.PushTime = attr.Value
	} else {
		userConfig.PushTime = "08:00" // 預設值
	}

	// Extract timezone
	if attr, ok := result.Item["timezone"].(*types.AttributeValueMemberS); ok {
		userConfig.Timezone = attr.Value
	} else {
		userConfig.Timezone = "Asia/Taipei" // 預設值
	}

	// Extract updatedAt
	if attr, ok := result.Item["updatedAt"].(*types.AttributeValueMemberS); ok {
		userConfig.UpdatedAt = attr.Value
	}

	return &userConfig, nil
}

func (r *userConfigRepository) GetUsersByCourse(course string) ([]models.UserConfig, error) {
	result, err := r.dynamodb.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("CourseIndex"), // GSI 名稱
		KeyConditionExpression: aws.String("course = :course"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":course": &types.AttributeValueMemberS{Value: course},
		},
	})

	if err != nil {
		r.logger.WithError(err).Error("Failed to query users by course from DynamoDB")
		return nil, fmt.Errorf("failed to query users by course: %w", err)
	}

	if result.Items == nil {
		return []models.UserConfig{}, nil
	}

	var userConfigs []models.UserConfig
	for _, item := range result.Items {
		var userConfig models.UserConfig

		// Extract userId
		if attr, ok := item["userId"].(*types.AttributeValueMemberS); ok {
			userConfig.UserID = attr.Value
		}

		// Extract course
		if attr, ok := item["course"].(*types.AttributeValueMemberS); ok {
			userConfig.Course = attr.Value
		}

		// Extract level
		if attr, ok := item["level"].(*types.AttributeValueMemberS); ok {
			level, err := strconv.Atoi(attr.Value)
			if err == nil {
				userConfig.Level = level
			}
		}

		// Extract updatedAt
		if attr, ok := item["updatedAt"].(*types.AttributeValueMemberS); ok {
			userConfig.UpdatedAt = attr.Value
		}

		userConfigs = append(userConfigs, userConfig)
	}

	r.logger.WithFields(logrus.Fields{
		"course": course,
		"count":  len(userConfigs),
	}).Info("Successfully retrieved users by course")

	return userConfigs, nil
}
