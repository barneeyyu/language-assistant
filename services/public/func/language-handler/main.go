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
	SERVICENAME = "language-handler"
)

type EnvVars struct {
	channelSecret       string
	channelToken        string
	openaiBaseUrl       string
	openaiApiKey        string
	vocabularyTableName string
	userTableName       string
}

func getEnvironmentVariables() (envVars *EnvVars, err error) {
	channelSecret := os.Getenv("CHANNEL_SECRET")
	if channelSecret == "" {
		return nil, errors.New("CHANNEL_SECRET is not set")
	}

	channelToken := os.Getenv("CHANNEL_TOKEN")
	if channelToken == "" {
		return nil, errors.New("CHANNEL_TOKEN is not set")
	}

	openaiBaseUrl := os.Getenv("OPENAI_BASE_URL")
	if openaiBaseUrl == "" {
		return nil, errors.New("OPENAI_BASE_URL is not set")
	}

	openaiApiKey := os.Getenv("OPENAI_API_KEY")
	if openaiApiKey == "" {
		return nil, errors.New("OPENAI_API_KEY is not set")
	}

	vocabularyTableName := os.Getenv("VOCABULARY_TABLE_NAME")
	if vocabularyTableName == "" {
		return nil, errors.New("VOCABULARY_TABLE_NAME is not set")
	}

	userTableName := os.Getenv("USER_TABLE_NAME")
	if userTableName == "" {
		return nil, errors.New("USER_TABLE_NAME is not set")
	}

	return &EnvVars{
		channelSecret:       channelSecret,
		channelToken:        channelToken,
		openaiBaseUrl:       openaiBaseUrl,
		openaiApiKey:        openaiApiKey,
		vocabularyTableName: vocabularyTableName,
		userTableName:       userTableName,
	}, nil
}

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  TIMESTAMP,
			logrus.FieldKeyLevel: SEVERITY,
			logrus.FieldKeyMsg:   MESSAGE,
		},
	})
	logger := logrus.WithField(COMPONENT, SERVICENAME)

	envVars, err := getEnvironmentVariables()
	if err != nil {
		logger.WithError(err).Error("Failed to get environment variables")
		panic(err)
	}

	linebotClient, err := utils.NewLineBotClient(envVars.channelSecret, envVars.channelToken)
	if err != nil {
		logger.WithError(err).Error("Failed to initialize LINE Bot")
		panic(err)
	}

	openaiClient, err := utils.NewOpenAIClient(envVars.openaiApiKey, envVars.openaiBaseUrl)
	if err != nil {
		panic(err)
	}

	// create EventBridge Scheduler client
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		panic(err)
	}
	dynamodbClient := dynamodb.NewFromConfig(cfg)

	vocabularyRepo := repository.NewVocabularyRepository(logger, dynamodbClient, envVars.vocabularyTableName)
	userConfigRepo := repository.NewUserConfigRepository(logger, dynamodbClient, envVars.userTableName)

	handler, err := NewHandler(logger, envVars, linebotClient, openaiClient, vocabularyRepo, userConfigRepo)
	if err != nil {
		logger.WithError(err).Error("Failed to create handler")
		panic(err)
	}

	lambda.Start(handler.EventHandler)
}
