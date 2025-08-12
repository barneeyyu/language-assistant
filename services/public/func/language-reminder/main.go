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
	SERVICENAME = "language-reminder"
)

type EnvVars struct {
	vocabularyTableName string
}

func getEnvironmentVariables() (envVars *EnvVars, err error) {
	vocabularyTableName := os.Getenv("VOCABULARY_TABLE_NAME")
	if vocabularyTableName == "" {
		return nil, errors.New("VOCABULARY_TABLE_NAME is not set")
	}

	return &EnvVars{
		vocabularyTableName: vocabularyTableName,
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

	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		panic(err)
	}
	dynamodbClient := dynamodb.NewFromConfig(cfg)

	reminderRepo := repository.NewReminderRepository(logger, dynamodbClient, envVars.vocabularyTableName)

	// Get environment variables for LINE Bot
	channelSecret := os.Getenv("CHANNEL_SECRET")
	if channelSecret == "" {
		panic(errors.New("CHANNEL_SECRET is not set"))
	}

	channelToken := os.Getenv("CHANNEL_TOKEN")
	if channelToken == "" {
		panic(errors.New("CHANNEL_TOKEN is not set"))
	}

	linebotClient, err := utils.NewLineBotClient(channelSecret, channelToken)
	if err != nil {
		logger.WithError(err).Error("Failed to create LINE Bot client")
		panic(err)
	}

	handler, err := NewHandler(logger, envVars, reminderRepo, linebotClient)
	if err != nil {
		logger.WithError(err).Error("Failed to create handler")
		panic(err)
	}

	lambda.Start(handler.EventHandler)
}
