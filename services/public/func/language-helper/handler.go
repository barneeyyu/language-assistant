package main

import (
	"bytes"
	"encoding/json"
	"language-assistant/internal/utils"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/line/line-bot-sdk-go/v7/linebot"
	"github.com/sirupsen/logrus"
)

type Handler struct {
	logger         *logrus.Entry
	envVars        *EnvVars
	linebotClient  utils.LinebotAPI
	openaiClient   utils.OpenaiAPI
	vocabularyRepo utils.VocabularyRepository
}

func NewHandler(logger *logrus.Entry, envVars *EnvVars, linebotClient utils.LinebotAPI, openaiClient utils.OpenaiAPI, vocabularyRepo utils.VocabularyRepository) (*Handler, error) {
	return &Handler{
		logger:         logger,
		envVars:        envVars,
		linebotClient:  linebotClient,
		openaiClient:   openaiClient,
		vocabularyRepo: vocabularyRepo,
	}, nil
}

func (h *Handler) EventHandler(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	h.logger.Info("Received request: ", request)

	messageEvents, err := h.RequestParser(request)
	if err != nil {
		h.logger.Error("Failed to parse request: ", err)
		return events.APIGatewayProxyResponse{
			StatusCode: 400,
			Body:       "Bad Request",
		}, nil
	}

	// Process each message event
	for _, event := range messageEvents {
		h.logger.WithFields(logrus.Fields{
			"event_type": event.Type,
			"user_id":    event.Source.UserID,
			"room_id":    event.Source.RoomID,
			"group_id":   event.Source.GroupID,
		}).Info("event handling")
		// Check if it's a message event
		if event.Type == linebot.EventTypeMessage {
			switch message := event.Message.(type) {
			case *linebot.TextMessage:
				translationResponse, err := h.openaiClient.Translate(message.Text)
				if err != nil {
					h.logger.WithError(err).Error("Failed to translate valid text")
					return events.APIGatewayProxyResponse{
						Body:       err.Error(),
						StatusCode: 500,
					}, nil
				}
				h.logger.Info("Translation response: ", translationResponse)

				for _, translation := range translationResponse.Translations {
					if err := h.vocabularyRepo.SaveWord(translation.Word, translation.PartOfSpeech, translation.Meaning, translation.Example.En, event.Source.UserID); err != nil {
						h.logger.Error("Failed to save word: ", err)
						continue
					}
				}
				// Reply with the same message
				if err := h.linebotClient.ReplyMessage(event.ReplyToken, translationResponse.String()); err != nil {
					h.logger.Error("Failed to reply message: ", err)
					continue
				}
			}
		}
	}

	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       "OK",
	}, nil
}

func (h *Handler) RequestParser(request events.APIGatewayProxyRequest) ([]*linebot.Event, error) {
	var bodyJSON interface{}
	if err := json.Unmarshal([]byte(request.Body), &bodyJSON); err != nil {
		h.logger.WithError(err).Error("Failed to parse JSON")
		return nil, err
	} else {
		h.logger.WithFields(logrus.Fields{
			"webhook_body": bodyJSON,
		}).Info("Received LINE webhook")
	}

	// analyze request body
	reqBody := bytes.NewBufferString(request.Body)
	req, err := http.NewRequest(http.MethodPost, "", reqBody)
	if err != nil {
		return nil, err
	}

	// parse all headers
	req.Header = make(http.Header)
	for key, value := range request.Headers {
		req.Header.Set(key, value)
	}
	// Parse the webhook event
	messageEvents, err := h.linebotClient.ParseRequest(req)
	if err != nil {
		h.logger.Error("Failed to parse webhook request: ", err)
		return nil, err
	}

	return messageEvents, nil
}
