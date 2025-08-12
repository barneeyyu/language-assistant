package main

import (
	"context"
	"errors"
	"language-assistant/internal/repository"
	"language-assistant/internal/utils"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/sirupsen/logrus"
)

const (
	SEVERITY    = "severity"
	MESSAGE     = "message"
	TIMESTAMP   = "timestamp"
	COMPONENT   = "component"
	SERVICENAME = "language-vocabulary"
)

type EnvVars struct {
	openaiBaseUrl       string
	openaiApiKey        string
	userTableName       string
	vocabularyTableName string
	channelToken        string
	channelSecret       string
}

func getEnvVars() (*EnvVars, error) {
	openaiBaseUrl := os.Getenv("OPENAI_BASE_URL")
	if openaiBaseUrl == "" {
		return nil, errors.New("OPENAI_BASE_URL is not set")
	}

	openaiApiKey := os.Getenv("OPENAI_API_KEY")
	if openaiApiKey == "" {
		return nil, errors.New("OPENAI_API_KEY is not set")
	}

	userTableName := os.Getenv("USER_TABLE_NAME")
	if userTableName == "" {
		return nil, errors.New("USER_TABLE_NAME is not set")
	}

	vocabularyTableName := os.Getenv("VOCABULARY_TABLE_NAME")
	if vocabularyTableName == "" {
		return nil, errors.New("VOCABULARY_TABLE_NAME is not set")
	}

	channelToken := os.Getenv("CHANNEL_TOKEN")
	if channelToken == "" {
		return nil, errors.New("CHANNEL_TOKEN is not set")
	}

	channelSecret := os.Getenv("CHANNEL_SECRET")
	if channelSecret == "" {
		return nil, errors.New("CHANNEL_SECRET is not set")
	}

	return &EnvVars{
		openaiBaseUrl:       openaiBaseUrl,
		openaiApiKey:        openaiApiKey,
		userTableName:       userTableName,
		vocabularyTableName: vocabularyTableName,
		channelToken:        channelToken,
		channelSecret:       channelSecret,
	}, nil
}

var handler *Handler

func init() {
	logrus.SetFormatter(&logrus.JSONFormatter{
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  TIMESTAMP,
			logrus.FieldKeyLevel: SEVERITY,
			logrus.FieldKeyMsg:   MESSAGE,
		},
	})
	logger := logrus.WithField(COMPONENT, SERVICENAME)

	envVars, err := getEnvVars()
	if err != nil {
		logger.WithError(err).Error("Failed to get environment variables")
		panic(err)
	}

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		logger.WithError(err).Error("Failed to load AWS config")
		panic(err)
	}

	dynamodbClient := dynamodb.NewFromConfig(cfg)

	openaiClient, err := utils.NewOpenAIClient(envVars.openaiApiKey, envVars.openaiBaseUrl)
	if err != nil {
		panic(err)
	}

	linebotClient, err := utils.NewLineBotClient(envVars.channelSecret, envVars.channelToken)
	if err != nil {
		panic(err)
	}

	userConfigRepo := repository.NewUserConfigRepository(logger, dynamodbClient, envVars.userTableName)
	bloomFilterRepo := repository.NewBloomFilterRepository(logger, dynamodbClient, envVars.vocabularyTableName)

	handler, err = NewHandler(logger, envVars, openaiClient, linebotClient, userConfigRepo, bloomFilterRepo)
	if err != nil {
		logger.WithError(err).Error("Failed to create handler")
		panic(err)
	}
}

// HandleRequest 處理直接 Lambda invoke（JSON payload）
func HandleRequest(ctx context.Context, request map[string]string) (map[string]interface{}, error) {
	return handler.HandleWordPush(request)
}

func main() {
	lambda.Start(HandleRequest)
}
