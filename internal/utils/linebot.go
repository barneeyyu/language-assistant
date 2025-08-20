package utils

import (
	"fmt"
	"net/http"

	"github.com/line/line-bot-sdk-go/v7/linebot"
)

type LinebotAPI interface {
	ReplyMessage(replyToken string, message string) error
	ReplyMessageWithMultiple(replyToken string, messages ...linebot.SendingMessage) error
	ParseRequest(req *http.Request) ([]*linebot.Event, error)
	PushMessage(userID string, message string) error
	GetProfile(userID string) (*linebot.UserProfileResponse, error)
}

type LineBotClient struct {
	client *linebot.Client
}

func NewLineBotClient(channelSecret string, channelToken string) (LinebotAPI, error) {
	client, err := linebot.New(channelSecret, channelToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create line bot client: %w", err)
	}
	return &LineBotClient{
		client: client,
	}, nil
}

func (c *LineBotClient) ReplyMessage(replyToken string, message string) error {
	_, err := c.client.ReplyMessage(replyToken, linebot.NewTextMessage(message)).Do()
	return err
}

func (c *LineBotClient) ReplyMessageWithMultiple(replyToken string, messages ...linebot.SendingMessage) error {
	_, err := c.client.ReplyMessage(replyToken, messages...).Do()
	return err
}

func (c *LineBotClient) ParseRequest(req *http.Request) ([]*linebot.Event, error) {
	return c.client.ParseRequest(req)
}

func (c *LineBotClient) PushMessage(userID string, message string) error {
	_, err := c.client.PushMessage(userID, linebot.NewTextMessage(message)).Do()
	return err
}

func (c *LineBotClient) GetProfile(userID string) (*linebot.UserProfileResponse, error) {
	return c.client.GetProfile(userID).Do()
}
