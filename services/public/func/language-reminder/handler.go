package main

import (
	"context"
	"encoding/json"
	"language-assistant/internal/models"
	"language-assistant/internal/utils"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/line/line-bot-sdk-go/v7/linebot"
	"github.com/sirupsen/logrus"
)

type Handler struct {
	logger       *logrus.Entry
	envVars      *EnvVars
	reminderRepo utils.ReminderRepository
}

type ReminderEvent struct {
	Date string `json:"date"`
}

func NewHandler(logger *logrus.Entry, envVars *EnvVars, reminderRepo utils.ReminderRepository) (*Handler, error) {
	return &Handler{
		logger:       logger,
		envVars:      envVars,
		reminderRepo: reminderRepo,
	}, nil
}

func (h *Handler) EventHandler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if req.Body != "" {
		var event ReminderEvent
		if err := json.Unmarshal([]byte(req.Body), &event); err != nil {
			h.logger.WithError(err).Error("Failed to decode reminder event")
			return events.APIGatewayProxyResponse{
				StatusCode: 400,
				Body:       "Bad request",
			}, nil
		}
		h.logger.Info("Getting the request for date: ", event.Date)
	}

	date := time.Now().Format("2006-01-02")
	userVoca, err := h.reminderRepo.GetUserVocabulariesByDate(date)
	if err != nil {
		h.logger.WithError(err).Error("Failed to get word")
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Body:       "Internal server error",
		}, nil
	}

	for index, dailyUserData := range userVoca {
		h.logger.Infof("Currently handle %d user: %s", index, dailyUserData.UserID)
		message := linebot.NewTextMessage(models.FormatWordRecords(dailyUserData.Words)).
			WithSender(&linebot.Sender{
				Name: "Reminder Bot",
			})
		if _, err := h.envVars.botClient.PushMessage(dailyUserData.UserID, message).Do(); err != nil {
			h.logger.WithError(err).Error("Failed to send reminder message")
			return events.APIGatewayProxyResponse{
				StatusCode: 500,
				Body:       "Internal server error",
			}, nil
		}
	}
	h.logger.Info("Successfully sent reminder message")
	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       "Reminder sent",
	}, nil
}

