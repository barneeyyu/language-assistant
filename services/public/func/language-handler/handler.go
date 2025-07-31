package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	userConfigRepo utils.UserConfigRepository
}

func NewHandler(logger *logrus.Entry, envVars *EnvVars, linebotClient utils.LinebotAPI, openaiClient utils.OpenaiAPI, vocabularyRepo utils.VocabularyRepository, userConfigRepo utils.UserConfigRepository) (*Handler, error) {
	return &Handler{
		logger:         logger,
		envVars:        envVars,
		linebotClient:  linebotClient,
		openaiClient:   openaiClient,
		vocabularyRepo: vocabularyRepo,
		userConfigRepo: userConfigRepo,
	}, nil
}

func (h *Handler) EventHandler(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
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

		if event.Type == linebot.EventTypeFollow {
			h.sendGreetingMessage(event.ReplyToken)
			continue
		}

		if event.Type == linebot.EventTypeMessage {
			switch message := event.Message.(type) {
			case *linebot.TextMessage:
				h.logger.WithField("text", message.Text).Info("Received text message")

				switch message.Text {
				case "/說明":
					h.sendGreetingMessage(event.ReplyToken)
					continue
				case "我對多益有興趣":
					h.handleCourseInterest(event.ReplyToken, event.Source.UserID, "toeic")
					continue
				case "我對雅思有興趣":
					h.handleCourseInterest(event.ReplyToken, event.Source.UserID, "ielts")
					continue
				default:
					// 檢查是否是數字（可能是分數輸入）
					if h.handleScoreInput(event.ReplyToken, event.Source.UserID, message.Text) {
						continue
					}

					// 原本的翻譯邏輯
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

func (h *Handler) sendGreetingMessage(replyToken string) {
	message := `👋 嗨！我是你的語言小幫手！

我可以幫你翻譯英文和中文，不論是英翻中還是中翻英，通通都沒問題 ✅  
而且我會在每天晚上幫你整理你今天問過的單字，協助你定期複習 🧠✨

如果你有興趣，也可以點選我們的字卡連結，我們目前支援「多益」與「雅思」的每日單字推播 📚📩

如有任何疑問，歡迎隨時輸入「/說明」來再次查看這份說明 📎`

	textMessage := linebot.NewTextMessage(message)

	// 再發送 CarouselTemplate
	template := linebot.NewCarouselTemplate(
		linebot.NewCarouselColumn(
			"", // 不使用圖片
			"📘 多益",
			"每天一字，幫助你準備 TOEIC！",
			linebot.NewMessageAction("有興趣", "我對多益有興趣"),
		),
		linebot.NewCarouselColumn(
			"",
			"📗 雅思",
			"提升你的 IELTS 單字力！",
			linebot.NewMessageAction("有興趣", "我對雅思有興趣"),
		),
	)
	templateMessage := linebot.NewTemplateMessage("字卡訂閱", template)
	if err := h.linebotClient.ReplyMessageWithMultiple(replyToken, textMessage, templateMessage); err != nil {
		h.logger.Error("Failed to send carousel template: ", err)
	}
}

func (h *Handler) handleCourseInterest(replyToken, userID, course string) {
	// 先儲存課程選擇（level 暫時設為 0，等待用戶輸入）
	if err := h.userConfigRepo.SaveUserConfig(userID, course, 0); err != nil {
		h.logger.WithError(err).Error("Failed to save user config")
		h.linebotClient.ReplyMessage(replyToken, "抱歉，設定過程發生錯誤，請稍後再試。")
		return
	}

	// 根據課程類型回覆不同訊息
	var message string
	if course == "toeic" {
		message = `太棒了！我已為你設定多益課程 📘

請告訴我你目前的多益分數（0-990分）：
如果不確定的話可以先隨機輸入一個大概的分數，之後如果難易度不符合可以再調整。

請直接輸入數字即可（例如：750）`
	} else {
		message = `太棒了！我已為你設定雅思課程 📗

請告訴我你目前的雅思分數（0-9分）：
如果不確定的話可以先隨機輸入一個大概的分數，之後如果難易度不符合可以再調整。

請直接輸入數字即可（例如：6.5）`
	}

	if err := h.linebotClient.ReplyMessage(replyToken, message); err != nil {
		h.logger.Error("Failed to reply course interest: ", err)
	}
}

func (h *Handler) handleScoreInput(replyToken, userID, text string) bool {
	// 檢查用戶是否有等待分數輸入的設定
	userConfig, err := h.userConfigRepo.GetUserConfig(userID)
	if err != nil {
		h.logger.WithError(err).Error("Failed to get user config")
		return false
	}

	// 如果沒有設定或分數已經設定過了，就不是分數輸入
	if userConfig == nil || userConfig.Level != 0 {
		return false
	}

	// 嘗試解析分數
	var score int
	var floatScore float64
	
	if userConfig.Course == "ielts" {
		// 雅思支援小數點輸入
		if _, err := fmt.Sscanf(text, "%f", &floatScore); err != nil {
			// 不是數字，不處理
			return false
		}
		// 轉換為整數存儲 (6.5 -> 65)
		score = int(floatScore * 10)
	} else {
		// 多益只接受整數
		if _, err := fmt.Sscanf(text, "%d", &score); err != nil {
			// 不是數字，不處理
			return false
		}
	}

	// 驗證分數範圍
	var isValid bool
	var message string

	if userConfig.Course == "toeic" {
		isValid = score >= 0 && score <= 990
		if isValid {
			message = fmt.Sprintf("✅ 已設定你的多益分數為 %d 分！\n\n現在你可以開始使用翻譯功能，我會根據你的程度提供合適的單字學習。", score)
		} else {
			message = "多益分數應該在 0-990 分之間，請重新輸入。"
		}
	} else { // ielts
		isValid = score >= 0 && score <= 90 // 0.0 到 9.0 分，轉換後是 0 到 90
		if isValid {
			realScore := float64(score) / 10.0
			message = fmt.Sprintf("✅ 已設定你的雅思分數為 %.1f 分！\n\n現在你可以開始使用翻譯功能，我會根據你的程度提供合適的單字學習。", realScore)
		} else {
			message = "雅思分數應該在 0-9 分之間（例如：6.5），請重新輸入。"
		}
	}

	if !isValid {
		h.linebotClient.ReplyMessage(replyToken, message)
		return true // 雖然分數無效，但確實是分數輸入嘗試
	}

	// 更新用戶設定
	if err := h.userConfigRepo.SaveUserConfig(userID, userConfig.Course, score); err != nil {
		h.logger.WithError(err).Error("Failed to update user config with score")
		h.linebotClient.ReplyMessage(replyToken, "抱歉，分數設定過程發生錯誤，請稍後再試。")
		return true
	}

	// 發送成功訊息
	if err := h.linebotClient.ReplyMessage(replyToken, message); err != nil {
		h.logger.Error("Failed to reply score confirmation: ", err)
	}

	return true
}
