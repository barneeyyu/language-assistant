package main

import (
	"context"
	"errors"
	"language-assistant/utils"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/scheduler"
	"github.com/sirupsen/logrus"
)

const (
	SEVERITY    = "severity"
	MESSAGE     = "message"
	TIMESTAMP   = "timestamp"
	COMPONENT   = "component"
	SERVICENAME = "language-helper"
)

type EnvVars struct {
	channelSecret       string
	channelToken        string
	openaiBaseUrl       string
	openaiApiKey        string
	ReminderFunctionArn string
	SchedulerRoleArn    string
	EventTableName      string
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

	eventTableName := os.Getenv("VOCABULARY_TABLE_NAME")
	if eventTableName == "" {
		return nil, errors.New("VOCABULARY_TABLE_NAME is not set")
	}

	return &EnvVars{
		channelSecret: channelSecret,
		channelToken:  channelToken,
		openaiBaseUrl: openaiBaseUrl,
		openaiApiKey:  openaiApiKey,
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
	schedulerClient := scheduler.NewFromConfig(cfg)
	dynamodbClient := dynamodb.NewFromConfig(cfg)

	handler, err := NewHandler(logger, envVars, linebotClient, openaiClient, schedulerClient, dynamodbClient)
	if err != nil {
		logger.WithError(err).Error("Failed to create handler")
		panic(err)
	}

	lambda.Start(handler.EventHandler)
}
