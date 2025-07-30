package utils

import (
	"fmt"
	"net/http"

	"github.com/line/line-bot-sdk-go/v7/linebot"
)

type LinebotAPI interface {
	ReplyMessage(replyToken string, message string) error
	ParseRequest(req *http.Request) ([]*linebot.Event, error)
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

func (c *LineBotClient) ParseRequest(req *http.Request) ([]*linebot.Event, error) {
	return c.client.ParseRequest(req)
}
