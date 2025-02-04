package main

import (
	"bytes"
	"context"
	"encoding/json"
	"language-assistant/utils"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/scheduler"
	"github.com/line/line-bot-sdk-go/v7/linebot"
	"github.com/sirupsen/logrus"
)

type DynamoDbAPI interface {
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
}

type Handler struct {
	logger          *logrus.Entry
	envVars         *EnvVars
	linebotClient   utils.LinebotAPI
	openaiClient    utils.OpenaiAPI
	schedulerClient *scheduler.Client
	dynamodbClient  DynamoDbAPI
}

func NewHandler(logger *logrus.Entry, envVars *EnvVars, linebotClient utils.LinebotAPI, openaiClient utils.OpenaiAPI, schedulerClient *scheduler.Client, dynamodbClient DynamoDbAPI) (*Handler, error) {
	return &Handler{
		logger:          logger,
		envVars:         envVars,
		linebotClient:   linebotClient,
		openaiClient:    openaiClient,
		schedulerClient: schedulerClient,
		dynamodbClient:  dynamodbClient,
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
					h.logger.WithError(err).Error("Failed to transfer valid schedule")
					return events.APIGatewayProxyResponse{
						Body:       err.Error(),
						StatusCode: 500,
					}, nil
				}
				h.logger.Info("Translation response: ", translationResponse)
				// Reply with the same message
				if err := h.linebotClient.ReplyMessage(event.ReplyToken, translationResponse); err != nil {
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
