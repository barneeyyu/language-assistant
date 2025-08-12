package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"language-assistant/internal/utils"
	"net/http"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
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
	lambdaClient   *lambda.Client
}

func NewHandler(logger *logrus.Entry, envVars *EnvVars, linebotClient utils.LinebotAPI, openaiClient utils.OpenaiAPI, vocabularyRepo utils.VocabularyRepository, userConfigRepo utils.UserConfigRepository, lambdaClient *lambda.Client) (*Handler, error) {
	return &Handler{
		logger:         logger,
		envVars:        envVars,
		linebotClient:  linebotClient,
		openaiClient:   openaiClient,
		vocabularyRepo: vocabularyRepo,
		userConfigRepo: userConfigRepo,
		lambdaClient:   lambdaClient,
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
				case "/設定推播":
					h.handlePushSettingsStart(event.ReplyToken)
					continue
				case "設定推播詳細":
					h.handlePushSettings(event.ReplyToken, event.Source.UserID)
					continue
				case "/使用預設設定":
					h.handleSkipPushSettings(event.ReplyToken, event.Source.UserID)
					continue
				default:
					// 檢查是否是推播設定相關的回應
					if h.handlePushSettingsResponse(event.ReplyToken, event.Source.UserID, message.Text) {
						continue
					}
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
不過目前暫時沒有興趣也沒關係，你可以隨時輸入「/設定推播」來開始設定。

如有任何疑問，歡迎隨時輸入「/說明」來再次查看這份說明 📎`

	textMessage := linebot.NewTextMessage(message)

	// 使用共用的 CarouselTemplate
	template := h.createCourseSelectionCarousel()
	templateMessage := linebot.NewTemplateMessage("字卡訂閱", template)
	if err := h.linebotClient.ReplyMessageWithMultiple(replyToken, textMessage, templateMessage); err != nil {
		h.logger.Error("Failed to send carousel template: ", err)
	}
}

func (h *Handler) handleCourseInterest(replyToken, userID, course string) {
	// 先儲存課程選擇（level 暫時設為 0，等待用戶輸入，使用預設的推播設定）
	if err := h.userConfigRepo.SaveUserConfig(userID, course, 0, 0, "", ""); err != nil {
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
			message = fmt.Sprintf("✅ 已設定你的多益分數為 %d 分！", score)
		} else {
			message = "多益分數應該在 0-990 分之間，請重新輸入。"
		}
	} else { // ielts
		isValid = score >= 0 && score <= 90 // 0.0 到 9.0 分，轉換後是 0 到 90
		if isValid {
			realScore := float64(score) / 10.0
			message = fmt.Sprintf("✅ 已設定你的雅思分數為 %.1f 分！", realScore)
		} else {
			message = "雅思分數應該在 0-9 分之間（例如：6.5），請重新輸入。"
		}
	}

	if !isValid {
		h.linebotClient.ReplyMessage(replyToken, message)
		return true // 雖然分數無效，但確實是分數輸入嘗試
	}

	// 更新用戶設定
	if err := h.userConfigRepo.SaveUserConfig(userID, userConfig.Course, score, 0, "", ""); err != nil {
		h.logger.WithError(err).Error("Failed to update user config with score")
		h.linebotClient.ReplyMessage(replyToken, "抱歉，分數設定過程發生錯誤，請稍後再試。")
		return true
	}

	// 發送成功訊息，並詢問是否要設定推播選項
	h.sendPushSettingsPrompt(replyToken, message)

	return true
}

func (h *Handler) sendPushSettingsPrompt(replyToken, scoreMessage string) {
	message := scoreMessage + "\n\n📱 要設定每日單字推播嗎？\n\n🔧 預設設定：每天10個單字，早上8:00推播\n❗ 如使用預設設定可直接跳過，並於明天開始推播~"

	textMessage := linebot.NewTextMessage(message)

	// 使用 Quick Reply 按鈕
	quickReply := linebot.NewQuickReplyItems(
		linebot.NewQuickReplyButton("", linebot.NewMessageAction("設定推播", "設定推播詳細")),
		linebot.NewQuickReplyButton("", linebot.NewMessageAction("使用預設設定", "/使用預設設定")),
	)

	textMessageWithQuickReply := textMessage.WithQuickReplies(quickReply)

	if err := h.linebotClient.ReplyMessageWithMultiple(replyToken, textMessageWithQuickReply); err != nil {
		h.logger.Error("Failed to send push settings prompt: ", err)
	}
}

func (h *Handler) handlePushSettings(replyToken, userID string) {
	// 獲取用戶當前設定，檢查是否已有課程
	userConfig, err := h.userConfigRepo.GetUserConfig(userID)
	if err != nil {
		h.logger.WithError(err).Error("Failed to get user config")
		h.linebotClient.ReplyMessage(replyToken, "抱歉，設定過程發生錯誤，請稍後再試。")
		return
	}

	if userConfig != nil && userConfig.Course != "" {
		// 用戶已有課程設定，直接進入單字量選擇
		var courseName string
		if userConfig.Course == "toeic" {
			courseName = "多益"
		} else {
			courseName = "雅思"
		}

		message := fmt.Sprintf("📱 設定 %s 推播詳細選項\n\n請選擇每天要收到幾個單字：", courseName)

		textMessage := linebot.NewTextMessage(message)

		// 單字量選擇的 Quick Reply
		quickReply := linebot.NewQuickReplyItems(
			linebot.NewQuickReplyButton("", linebot.NewMessageAction("5個單字", "單字量:5")),
			linebot.NewQuickReplyButton("", linebot.NewMessageAction("10個單字", "單字量:10")),
			linebot.NewQuickReplyButton("", linebot.NewMessageAction("15個單字", "單字量:15")),
			linebot.NewQuickReplyButton("", linebot.NewMessageAction("20個單字", "單字量:20")),
		)

		textMessageWithQuickReply := textMessage.WithQuickReplies(quickReply)

		// 暫存用戶已有的課程
		h.tempStoreCourse(userID, userConfig.Course)

		if err := h.linebotClient.ReplyMessageWithMultiple(replyToken, textMessageWithQuickReply); err != nil {
			h.logger.Error("Failed to send daily words selection: ", err)
		}
	} else {
		// 用戶沒有課程設定，顯示課程選擇
		h.handlePushSettingsStart(replyToken)
	}
}

func (h *Handler) handleSkipPushSettings(replyToken, userID string) {
	// 獲取用戶當前設定
	userConfig, err := h.userConfigRepo.GetUserConfig(userID)
	if err != nil {
		h.logger.WithError(err).Error("Failed to get user config")
		h.linebotClient.ReplyMessage(replyToken, "抱歉，設定過程發生錯誤，請稍後再試。")
		return
	}

	if userConfig == nil {
		h.linebotClient.ReplyMessage(replyToken, "請先設定課程和分數。")
		return
	}

	// 使用預設設定：10個單字，早上8:00推播
	userConfig.DailyWords = 10          // 預設每日單字數量
	userConfig.PushTime = "08:00"       // 預設推播時間
	userConfig.Timezone = "Asia/Taipei" // 預設時區

	// 使用預設設定：10個單字，早上8:00推播
	if err := h.userConfigRepo.SaveUserConfig(userID, userConfig.Course, userConfig.Level, userConfig.DailyWords, userConfig.PushTime, userConfig.Timezone); err != nil {
		h.logger.WithError(err).Error("Failed to save default push settings")
		h.linebotClient.ReplyMessage(replyToken, "抱歉，設定過程發生錯誤，請稍後再試。")
		return
	}

	var courseName string
	if userConfig.Course == "toeic" {
		courseName = "多益"
	} else {
		courseName = "雅思"
	}

	message := fmt.Sprintf("🎉 已使用預設推播設定！\n\n📱 你的推播設定：\n• 課程：%s\n• 每天 10 個單字\n• 推播時間：08:00\n\n🚀 馬上為您推播 %s 單字，下一次會於明天 08:00 推播！\n\n現在你可以開始使用翻譯功能！", courseName, courseName)

	// 立即推播第一次單字
	go h.triggerImmediateWordPush(userID)

	if err := h.linebotClient.ReplyMessage(replyToken, message); err != nil {
		h.logger.Error("Failed to send default settings confirmation: ", err)
	}
}

func (h *Handler) handlePushSettingsResponse(replyToken, userID, text string) bool {
	h.logger.WithField("text", text).Info("Checking push settings response")

	// 檢查是否是推播設定的課程選擇
	if strings.HasPrefix(text, "推播設定:") {
		h.logger.Info("Matched 推播設定 prefix")
		courseStr := strings.TrimPrefix(text, "推播設定:")
		h.logger.WithField("course", courseStr).Info("Extracted course")

		if courseStr == "toeic" || courseStr == "ielts" {
			h.handlePushSettingsCourseSelected(replyToken, userID, courseStr)
			return true
		}
		return false
	}

	// 檢查是否是單字量設定
	if strings.HasPrefix(text, "單字量:") {
		h.logger.Info("Matched 單字量 prefix")
		dailyWordsStr := strings.TrimPrefix(text, "單字量:")
		h.logger.WithField("dailyWordsStr", dailyWordsStr).Info("Extracted daily words string")

		dailyWords := 0
		switch dailyWordsStr {
		case "5":
			dailyWords = 5
		case "10":
			dailyWords = 10
		case "15":
			dailyWords = 15
		case "20":
			dailyWords = 20
		default:
			h.logger.WithField("dailyWordsStr", dailyWordsStr).Warn("Unknown daily words value")
			return false
		}

		h.logger.WithField("dailyWords", dailyWords).Info("Processing daily words selection")
		h.handleDailyWordsSelection(replyToken, userID, dailyWords)
		return true
	}

	// 檢查是否是推播時間設定
	if strings.HasPrefix(text, "時間:") {
		h.logger.Info("Matched 時間 prefix")
		pushTime := strings.TrimPrefix(text, "時間:")
		h.logger.WithField("pushTime", pushTime).Info("Extracted push time")
		h.handlePushTimeSelection(replyToken, userID, pushTime)
		return true
	}

	h.logger.Info("No push settings pattern matched")
	return false
}

func (h *Handler) handleDailyWordsSelection(replyToken, userID string, dailyWords int) {
	message := fmt.Sprintf("✅ 已設定每天推播 %d 個單字\n\n請選擇推播時間：", dailyWords)

	textMessage := linebot.NewTextMessage(message)

	// 推播時間選擇的 Quick Reply
	quickReply := linebot.NewQuickReplyItems(
		linebot.NewQuickReplyButton("", linebot.NewMessageAction("早上 8:00", "時間:08:00")),
		linebot.NewQuickReplyButton("", linebot.NewMessageAction("中午 12:00", "時間:12:00")),
		linebot.NewQuickReplyButton("", linebot.NewMessageAction("晚上 7:00", "時間:19:00")),
	)

	textMessageWithQuickReply := textMessage.WithQuickReplies(quickReply)

	// 暫存用戶選擇的單字量
	h.tempStoreDailyWords(userID, dailyWords)

	if err := h.linebotClient.ReplyMessageWithMultiple(replyToken, textMessageWithQuickReply); err != nil {
		h.logger.Error("Failed to send push time selection: ", err)
	}
}

func (h *Handler) handlePushTimeSelection(replyToken, userID, pushTime string) {
	// 獲取臨時存儲的單字量和課程
	dailyWords := h.getTempDailyWords(userID)
	if dailyWords == 0 {
		dailyWords = 10 // 預設值
	}

	tempCourse := h.getTempCourse(userID)

	// 如果有暫存的課程，表示這是從推播設定流程來的
	if tempCourse != "" {
		h.logger.Info("Handling push settings flow")

		// 檢查用戶是否已有設定
		userConfig, err := h.userConfigRepo.GetUserConfig(userID)
		if err != nil {
			h.logger.WithError(err).Error("Failed to get user config")
			h.linebotClient.ReplyMessage(replyToken, "抱歉，設定過程發生錯誤，請稍後再試。")
			return
		}

		// 確定要使用的 level
		level := 0 // 預設 level
		if userConfig != nil {
			level = userConfig.Level // 使用現有的 level
		}

		// 更新推播設定
		if err := h.userConfigRepo.SaveUserConfig(userID, tempCourse, level, dailyWords, pushTime, "Asia/Taipei"); err != nil {
			h.logger.WithError(err).Error("Failed to update user config with push settings")
			h.linebotClient.ReplyMessage(replyToken, "抱歉，設定過程發生錯誤，請稍後再試。")
			return
		}

		// 清理臨時存儲
		h.clearTempDailyWords(userID)
		h.clearTempCourse(userID)

		var courseName string
		if tempCourse == "toeic" {
			courseName = "多益"
		} else {
			courseName = "雅思"
		}

		message := fmt.Sprintf("🎉 推播設定完成！\n\n📱 你的推播設定：\n• 課程：%s\n• 每天 %d 個單字\n• 推播時間：%s\n\n🚀 馬上為您推播 %s 單字，下一次會於明天 %s 推播！\n\n現在你可以開始使用翻譯功能！", courseName, dailyWords, pushTime, courseName, pushTime)

		// 立即推播第一次單字
		go h.triggerImmediateWordPush(userID)

		if err := h.linebotClient.ReplyMessage(replyToken, message); err != nil {
			h.logger.Error("Failed to send push settings confirmation: ", err)
		}
		return
	}

	// 原來的邏輯：分數設定後的推播設定
	userConfig, err := h.userConfigRepo.GetUserConfig(userID)
	if err != nil {
		h.logger.WithError(err).Error("Failed to get user config")
		h.linebotClient.ReplyMessage(replyToken, "抱歉，設定過程發生錯誤，請稍後再試。")
		return
	}

	if userConfig == nil {
		h.linebotClient.ReplyMessage(replyToken, "請先設定課程和分數。")
		return
	}

	// 更新用戶設定
	if err := h.userConfigRepo.SaveUserConfig(userID, userConfig.Course, userConfig.Level, dailyWords, pushTime, "Asia/Taipei"); err != nil {
		h.logger.WithError(err).Error("Failed to update user config with push settings")
		h.linebotClient.ReplyMessage(replyToken, "抱歉，設定過程發生錯誤，請稍後再試。")
		return
	}

	// 清理臨時存儲
	h.clearTempDailyWords(userID)

	var courseName string
	if userConfig.Course == "toeic" {
		courseName = "多益"
	} else {
		courseName = "雅思"
	}

	message := fmt.Sprintf("🎉 推播設定完成！\n\n📱 你的推播設定：\n• 每天 %d 個單字\n• 推播時間：%s\n\n🚀 馬上為您推播 %s 單字，下一次會於明天 %s 推播！\n\n現在你可以開始使用翻譯功能，我會根據你的程度提供合適的單字學習。", dailyWords, pushTime, courseName, pushTime)

	// 立即推播第一次單字
	go h.triggerImmediateWordPush(userID)

	if err := h.linebotClient.ReplyMessage(replyToken, message); err != nil {
		h.logger.Error("Failed to send push settings confirmation: ", err)
	}
}

// 臨時存儲機制（簡單實現，生產環境可能需要 Redis 或其他方案）
var tempDailyWordsStorage = make(map[string]int)
var tempCourseStorage = make(map[string]string)

func (h *Handler) tempStoreDailyWords(userID string, dailyWords int) {
	tempDailyWordsStorage[userID] = dailyWords
}

func (h *Handler) getTempDailyWords(userID string) int {
	return tempDailyWordsStorage[userID]
}

func (h *Handler) clearTempDailyWords(userID string) {
	delete(tempDailyWordsStorage, userID)
}

func (h *Handler) tempStoreCourse(userID string, course string) {
	tempCourseStorage[userID] = course
}

func (h *Handler) getTempCourse(userID string) string {
	return tempCourseStorage[userID]
}

func (h *Handler) clearTempCourse(userID string) {
	delete(tempCourseStorage, userID)
}

func (h *Handler) handlePushSettingsCourseSelected(replyToken, userID, course string) {
	var courseName string
	if course == "toeic" {
		courseName = "多益"
	} else {
		courseName = "雅思"
	}

	message := fmt.Sprintf("✅ 已選擇 %s 字卡\n\n📱 設定每日推播\n\n請選擇每天要收到幾個單字：", courseName)

	textMessage := linebot.NewTextMessage(message)

	// 單字量選擇的 Quick Reply
	quickReply := linebot.NewQuickReplyItems(
		linebot.NewQuickReplyButton("", linebot.NewMessageAction("5個單字", "單字量:5")),
		linebot.NewQuickReplyButton("", linebot.NewMessageAction("10個單字", "單字量:10")),
		linebot.NewQuickReplyButton("", linebot.NewMessageAction("15個單字", "單字量:15")),
		linebot.NewQuickReplyButton("", linebot.NewMessageAction("20個單字", "單字量:20")),
	)

	textMessageWithQuickReply := textMessage.WithQuickReplies(quickReply)

	// 暫存用戶選擇的課程
	h.tempStoreCourse(userID, course)

	if err := h.linebotClient.ReplyMessageWithMultiple(replyToken, textMessageWithQuickReply); err != nil {
		h.logger.Error("Failed to send daily words selection for push settings: ", err)
	}
}

// 創建課程選擇的 CarouselTemplate
func (h *Handler) createCourseSelectionCarousel() *linebot.CarouselTemplate {
	var toeicAction, ieltsAction linebot.TemplateAction

	toeicAction = linebot.NewMessageAction("有興趣", "我對多益有興趣")
	ieltsAction = linebot.NewMessageAction("有興趣", "我對雅思有興趣")

	var toeicDesc, ieltsDesc string
	toeicDesc = "每天一字，幫助你準備 TOEIC！"
	ieltsDesc = "提升你的 IELTS 單字力！"

	return linebot.NewCarouselTemplate(
		linebot.NewCarouselColumn(
			"", // 不使用圖片
			"📘 多益",
			toeicDesc,
			toeicAction,
		),
		linebot.NewCarouselColumn(
			"",
			"📗 雅思",
			ieltsDesc,
			ieltsAction,
		),
	)
}

func (h *Handler) handlePushSettingsStart(replyToken string) {
	message := `📱 設定每日單字推播

請選擇你想要的字卡類型：`

	textMessage := linebot.NewTextMessage(message)

	// 使用共用的 CarouselTemplate
	template := h.createCourseSelectionCarousel()
	templateMessage := linebot.NewTemplateMessage("字卡類型選擇", template)

	if err := h.linebotClient.ReplyMessageWithMultiple(replyToken, textMessage, templateMessage); err != nil {
		h.logger.Error("Failed to send push settings course selection: ", err)
	}
}

// triggerImmediateWordPush 立即invoke language-vocabulary lambda推播一次單字給用戶
func (h *Handler) triggerImmediateWordPush(userID string) {
	h.logger.Infof("Triggering immediate word push for user %s", userID)

	// 構造 lambda invoke 請求
	requestPayload := map[string]string{
		"userId": userID,
	}

	payloadBytes, err := json.Marshal(requestPayload)
	if err != nil {
		h.logger.WithError(err).Error("Failed to marshal lambda invoke payload")
		return
	}

	// Invoke language-vocabulary lambda
	input := &lambda.InvokeInput{
		FunctionName:   aws.String("language-vocabulary"), // Lambda function name
		InvocationType: "Event",                           // 異步調用，不等待回應
		Payload:        payloadBytes,
	}

	ctx := context.Background()
	_, err = h.lambdaClient.Invoke(ctx, input)
	if err != nil {
		h.logger.WithError(err).Error("Failed to invoke language-vocabulary lambda")
		return
	}

	h.logger.Infof("Successfully triggered immediate word push for user %s", userID)
}
