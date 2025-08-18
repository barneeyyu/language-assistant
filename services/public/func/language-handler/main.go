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
	lambdaService "github.com/aws/aws-sdk-go-v2/service/lambda"
	schedulerService "github.com/aws/aws-sdk-go-v2/service/scheduler"
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
	channelSecret         string
	channelToken          string
	openaiBaseUrl         string
	openaiApiKey          string
	vocabularyTableName   string
	userTableName         string
	vocabularyFunctionArn string
	schedulerRoleArn      string
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

	vocabularyFunctionArn := os.Getenv("VOCABULARY_FUNCTION_ARN")
	if vocabularyFunctionArn == "" {
		return nil, errors.New("VOCABULARY_FUNCTION_ARN is not set")
	}

	schedulerRoleArn := os.Getenv("SCHEDULER_ROLE_ARN")
	if schedulerRoleArn == "" {
		return nil, errors.New("SCHEDULER_ROLE_ARN is not set")
	}

	return &EnvVars{
		channelSecret:         channelSecret,
		channelToken:          channelToken,
		openaiBaseUrl:         openaiBaseUrl,
		openaiApiKey:          openaiApiKey,
		vocabularyTableName:   vocabularyTableName,
		userTableName:         userTableName,
		vocabularyFunctionArn: vocabularyFunctionArn,
		schedulerRoleArn:      schedulerRoleArn,
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

	// create AWS clients
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		panic(err)
	}
	dynamodbClient := dynamodb.NewFromConfig(cfg)
	lambdaClient := lambdaService.NewFromConfig(cfg)
	schedulerClient := schedulerService.NewFromConfig(cfg)

	vocabularyRepo := repository.NewVocabularyRepository(logger, dynamodbClient, envVars.vocabularyTableName)
	userConfigRepo := repository.NewUserConfigRepository(logger, dynamodbClient, envVars.userTableName)

	handler, err := NewHandler(logger, envVars, linebotClient, openaiClient, vocabularyRepo, userConfigRepo, lambdaClient, schedulerClient)
	if err != nil {
		logger.WithError(err).Error("Failed to create handler")
		panic(err)
	}

	lambda.Start(handler.EventHandler)
}
