package main

import (
	"context"
	"time"

	"language-assistant/internal/models"
	"language-assistant/internal/utils"

	"github.com/aws/aws-lambda-go/events"
	"github.com/sirupsen/logrus"
)

type Handler struct {
	logger        *logrus.Entry
	envVars       *EnvVars
	reminderRepo  utils.ReminderRepository
	linebotClient utils.LinebotAPI
}

func NewHandler(logger *logrus.Entry, envVars *EnvVars, reminderRepo utils.ReminderRepository, linebotClient utils.LinebotAPI) (*Handler, error) {
	return &Handler{
		logger:        logger,
		envVars:       envVars,
		reminderRepo:  reminderRepo,
		linebotClient: linebotClient,
	}, nil
}

func (h *Handler) EventHandler(ctx context.Context, event events.CloudWatchEvent) error {
	h.logger.WithFields(logrus.Fields{
		"source":     event.Source,
		"detailType": event.DetailType,
		"eventTime":  event.Time,
	}).Info("Daily reminder cron job triggered")

	date := time.Now().Format("2006-01-02")
	userVocaList, err := h.reminderRepo.GetUserVocabulariesByDate(date)
	if err != nil {
		h.logger.WithError(err).Error("Failed to get word")
		return err
	}

	// 如果沒有任何用戶有單字需要回顧，直接結束
	if len(userVocaList) == 0 {
		h.logger.WithField("date", date).Info("No users with vocabulary to review today, skipping reminder job")
		return nil
	}

	for index, dailyUserData := range userVocaList {
		h.logger.WithFields(logrus.Fields{
			"userIndex": index,
			"userID":    dailyUserData.UserID,
			"wordCount": len(dailyUserData.Words),
		}).Info("Sending daily reminder to user")

		messageText := models.FormatWordRecords(dailyUserData.Words)
		if err := h.linebotClient.PushMessage(dailyUserData.UserID, messageText); err != nil {
			h.logger.WithError(err).WithField("userID", dailyUserData.UserID).Error("Failed to send reminder message")
			continue // 繼續處理其他用戶，不要因為一個用戶失敗就中斷整個流程
		}
	}
	return nil
}
