package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"language-assistant/internal/models"
	"language-assistant/internal/utils"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/scheduler"
	"github.com/aws/aws-sdk-go-v2/service/scheduler/types"
	"github.com/line/line-bot-sdk-go/v7/linebot"
	"github.com/sirupsen/logrus"
)

type Handler struct {
	logger          *logrus.Entry
	envVars         *EnvVars
	linebotClient   utils.LinebotAPI
	openaiClient    utils.OpenaiAPI
	vocabularyRepo  utils.VocabularyRepository
	userConfigRepo  utils.UserConfigRepository
	lambdaClient    *lambda.Client
	schedulerClient *scheduler.Client
}

func NewHandler(logger *logrus.Entry, envVars *EnvVars, linebotClient utils.LinebotAPI, openaiClient utils.OpenaiAPI, vocabularyRepo utils.VocabularyRepository, userConfigRepo utils.UserConfigRepository, lambdaClient *lambda.Client, schedulerClient *scheduler.Client) (*Handler, error) {
	return &Handler{
		logger:          logger,
		envVars:         envVars,
		linebotClient:   linebotClient,
		openaiClient:    openaiClient,
		vocabularyRepo:  vocabularyRepo,
		userConfigRepo:  userConfigRepo,
		lambdaClient:    lambdaClient,
		schedulerClient: schedulerClient,
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
			h.handleUserFollow(event.ReplyToken, event.Source.UserID)
			continue
		}

		if event.Type == linebot.EventTypeMessage {
			switch message := event.Message.(type) {
			case *linebot.TextMessage:
				h.logger.WithField("text", message.Text).Info("Received text message")

				// 檢查用戶是否已有設定
				userConfig, err := h.userConfigRepo.GetUserConfig(event.Source.UserID)
				if err != nil {
					h.logger.WithError(err).Error("Failed to get user config")
				}

				switch message.Text {
				case "/說明":
					h.sendGreetingMessage(event.ReplyToken)
					continue
				case "我對多益有興趣":
					h.handleCourseInterest(event.ReplyToken, userConfig.DisplayName, event.Source.UserID, "toeic")
					continue
				case "我對雅思有興趣":
					h.handleCourseInterest(event.ReplyToken, userConfig.DisplayName, event.Source.UserID, "ielts")
					continue
				case "/設定推播":
					h.handlePushSettingsStart(event.ReplyToken)
					continue
				case "/設定推播詳細":
					h.handlePushSettings(event.ReplyToken, event.Source.UserID, userConfig)
					continue
				case "/使用預設設定":
					h.handleSkipPushSettings(event.ReplyToken, event.Source.UserID, userConfig)
					continue
				case "/個人設定":
					h.handleShowUserSettings(event.ReplyToken, event.Source.UserID)
					continue
				default:
					// 檢查是否是無效的 "/" 命令
					if strings.HasPrefix(message.Text, "/") {
						h.linebotClient.ReplyMessage(event.ReplyToken, "❌ 目前無此設定\n\n可使用的指令：\n• /說明 - 查看使用說明\n• /設定推播 - 設定推播選項\n• /個人設定 - 查看個人設定")
						continue
					}

					// 檢查是否是推播設定相關的回應
					if h.handlePushSettingsResponse(event.ReplyToken, event.Source.UserID, message.Text, userConfig) {
						continue
					}
					// 檢查是否是數字（可能是分數輸入）
					if h.handleScoreInput(event.ReplyToken, userConfig.DisplayName, event.Source.UserID, message.Text) {
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

func (h *Handler) handleUserFollow(replyToken, userID string) {
	h.logger.WithField("userID", userID).Info("User followed the bot")

	// 獲取用戶資料
	profile, err := h.linebotClient.GetProfile(userID)
	if err != nil {
		h.logger.WithError(err).WithField("userID", userID).Error("Failed to get user profile")
		// 即使獲取資料失敗，仍然發送歡迎訊息
		h.sendGreetingMessage(replyToken)
		return
	}

	displayName := profile.DisplayName
	h.logger.WithFields(logrus.Fields{
		"userID":      userID,
		"displayName": displayName,
	}).Info("Retrieved user profile")

	// 建立基本用戶記錄
	if err := h.userConfigRepo.SaveUserConfig(userID, displayName, "", 0, 0, "", ""); err != nil {
		h.logger.WithError(err).WithFields(logrus.Fields{
			"userID":      userID,
			"displayName": displayName,
		}).Error("Failed to create initial user record")
		// 即使建立記錄失敗，仍然發送歡迎訊息
	} else {
		h.logger.WithFields(logrus.Fields{
			"userID":      userID,
			"displayName": displayName,
		}).Info("Successfully created initial user record")
	}

	// 發送歡迎訊息
	h.sendGreetingMessage(replyToken)
}

func (h *Handler) sendGreetingMessage(replyToken string) {
	message := `👋 嗨！我是你的語言小幫手！

我可以幫你翻譯英文和中文，不論是英翻中還是中翻英，通通都沒問題 ✅  
而且我會在每天晚上幫你整理你今天問過的單字，協助你定期複習 🧠✨

如果你有興趣，也可以點選我們的字卡連結，我們目前支援「多益」與「雅思」的每日單字推播 📚📩
不過目前暫時沒有興趣也沒關係，你可以隨時輸入「/設定推播」來開始設定。
也可以輸入「/個人設定」來查看你的設定紀錄唷！

如有任何疑問，歡迎隨時輸入「/說明」來再次查看這份說明 📎`

	textMessage := linebot.NewTextMessage(message)

	// 使用共用的 CarouselTemplate
	template := h.createCourseSelectionCarousel()
	templateMessage := linebot.NewTemplateMessage("字卡訂閱", template)
	if err := h.linebotClient.ReplyMessageWithMultiple(replyToken, textMessage, templateMessage); err != nil {
		h.logger.Error("Failed to send carousel template: ", err)
	}
}

func (h *Handler) handleCourseInterest(replyToken, userName, userID, course string) {
	// 先儲存課程選擇（level 暫時設為 0，等待用戶輸入，使用預設的推播設定）
	if err := h.userConfigRepo.SaveUserConfig(userID, userName, course, 0, 0, "", ""); err != nil {
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

func (h *Handler) handleScoreInput(replyToken, userName, userID, text string) bool {
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
	if err := h.userConfigRepo.SaveUserConfig(userID, userName, userConfig.Course, score, 0, "", ""); err != nil {
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
		linebot.NewQuickReplyButton("", linebot.NewMessageAction("設定推播", "/設定推播詳細")),
		linebot.NewQuickReplyButton("", linebot.NewMessageAction("使用預設設定", "/使用預設設定")),
	)

	textMessageWithQuickReply := textMessage.WithQuickReplies(quickReply)

	if err := h.linebotClient.ReplyMessageWithMultiple(replyToken, textMessageWithQuickReply); err != nil {
		h.logger.Error("Failed to send push settings prompt: ", err)
	}
}

func (h *Handler) handleShowUserSettings(replyToken, userID string) {
	userConfig, err := h.userConfigRepo.GetUserConfig(userID)
	if err != nil {
		h.logger.WithError(err).Error("Failed to get user config")
		h.linebotClient.ReplyMessage(replyToken, "抱歉，無法取得您的設定資料，請稍後再試。")
		return
	}

	if userConfig == nil {
		h.linebotClient.ReplyMessage(replyToken, "📝 您尚未完成設定\n\n請先：\n1. 選擇課程（多益/雅思）\n2. 設定您的程度分數\n3. 設定推播選項\n\n💡 輸入「/說明」查看完整使用說明")
		return
	}

	// 格式化用戶設定資訊
	var message strings.Builder
	message.WriteString("⚙️ 個人設定資訊\n\n")

	// 顯示名稱
	if userConfig.DisplayName != "" {
		message.WriteString(fmt.Sprintf("👤 用戶名稱：%s\n", userConfig.DisplayName))
	}

	// 課程資訊
	if userConfig.Course != "" {
		var courseName, levelInfo string
		if userConfig.Course == "toeic" {
			courseName = "多益 (TOEIC)"
			if userConfig.Level > 0 {
				levelInfo = fmt.Sprintf("%d 分", userConfig.Level)
			}
		} else if userConfig.Course == "ielts" {
			courseName = "雅思 (IELTS)"
			if userConfig.Level > 0 {
				realScore := float64(userConfig.Level) / 10.0
				levelInfo = fmt.Sprintf("%.1f 分", realScore)
			}
		}
		message.WriteString(fmt.Sprintf("📚 課程：%s\n", courseName))

		if levelInfo != "" {
			message.WriteString(fmt.Sprintf("📊 程度：%s\n", levelInfo))
		} else {
			message.WriteString("📊 程度：尚未設定\n")
		}
	} else {
		message.WriteString("📚 課程：尚未選擇\n")
		message.WriteString("📊 程度：尚未設定\n")
	}

	// 推播設定
	if userConfig.DailyWords > 0 {
		message.WriteString(fmt.Sprintf("📱 每日推播：%d 個單字\n", userConfig.DailyWords))
	} else {
		message.WriteString("📱 每日推播：尚未設定\n")
	}

	if userConfig.PushTime != "" {
		message.WriteString(fmt.Sprintf("⏰ 推播時間：%s\n", userConfig.PushTime))
	} else {
		message.WriteString("⏰ 推播時間：尚未設定\n")
	}

	if userConfig.Timezone != "" {
		message.WriteString(fmt.Sprintf("🌏 時區：%s\n", userConfig.Timezone))
	}

	// 設定完成度檢查
	message.WriteString("\n")
	if userConfig.Course != "" && userConfig.Level > 0 && userConfig.DailyWords > 0 && userConfig.PushTime != "" {
		message.WriteString("✅ 設定已完成！\n\n💡 可使用「/設定推播」重新調整推播設定")
	} else {
		message.WriteString("⚠️ 設定尚未完整\n\n💡 使用「/設定推播」完成剩餘設定")
	}

	if err := h.linebotClient.ReplyMessage(replyToken, message.String()); err != nil {
		h.logger.Error("Failed to send user settings: ", err)
	}
}

func (h *Handler) handlePushSettings(replyToken, userID string, userConfig *models.UserConfig) {
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

func (h *Handler) handleSkipPushSettings(replyToken, userID string, userConfig *models.UserConfig) {
	if userConfig == nil {
		h.linebotClient.ReplyMessage(replyToken, "請先設定課程和分數。")
		return
	}

	// 使用預設設定：10個單字，早上8:00推播
	userConfig.DailyWords = 10          // 預設每日單字數量
	userConfig.PushTime = "08:00"       // 預設推播時間
	userConfig.Timezone = "Asia/Taipei" // 預設時區

	// 使用預設設定：10個單字，早上8:00推播
	if err := h.userConfigRepo.SaveUserConfig(userID, userConfig.DisplayName, userConfig.Course, userConfig.Level, userConfig.DailyWords, userConfig.PushTime, userConfig.Timezone); err != nil {
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

	// 設定推播排程並立即推播
	if err := h.setupUserPushSchedule(userID, userConfig.PushTime, userConfig.Timezone); err != nil {
		errorMessage := "⚠️ 排程建立失敗，請稍後重新設定或聯絡客服。"
		if replyErr := h.linebotClient.ReplyMessage(replyToken, errorMessage); replyErr != nil {
			h.logger.Error("Failed to send error message: ", replyErr)
		}
		return
	}

	if err := h.linebotClient.ReplyMessage(replyToken, message); err != nil {
		h.logger.Error("Failed to send default settings confirmation: ", err)
	}
}

func (h *Handler) handlePushSettingsResponse(replyToken, userID, text string, userConfig *models.UserConfig) bool {
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
		h.handlePushTimeSelection(replyToken, userID, pushTime, userConfig)
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

func (h *Handler) handlePushTimeSelection(replyToken, userID, pushTime string, userConfig *models.UserConfig) {
	// 獲取臨時存儲的單字量和課程
	dailyWords := h.getTempDailyWords(userID)
	if dailyWords == 0 {
		dailyWords = 10 // 預設值
	}

	tempCourse := h.getTempCourse(userID)

	// 確定最終的課程和等級
	var finalCourse string
	var finalLevel int
	var displayName string

	if tempCourse != "" {
		// 從推播設定流程來的
		finalCourse = tempCourse
		finalLevel = 0 // 預設 level
		if userConfig != nil {
			finalLevel = userConfig.Level
			displayName = userConfig.DisplayName
		}
		h.logger.Info("Handling push settings flow")
	} else {
		// 從分數設定後的推播設定來的，需要重新獲取用戶設定
		var err error
		userConfig, err = h.userConfigRepo.GetUserConfig(userID)
		if err != nil {
			h.logger.WithError(err).Error("Failed to get user config")
			h.linebotClient.ReplyMessage(replyToken, "抱歉，設定過程發生錯誤，請稍後再試。")
			return
		}

		if userConfig == nil {
			h.linebotClient.ReplyMessage(replyToken, "請先設定課程和分數。")
			return
		}

		finalCourse = userConfig.Course
		finalLevel = userConfig.Level
		displayName = userConfig.DisplayName
		h.logger.Info("Handling score input flow")
	}

	// 統一更新用戶設定
	if err := h.userConfigRepo.SaveUserConfig(userID, displayName, finalCourse, finalLevel, dailyWords, pushTime, "Asia/Taipei"); err != nil {
		h.logger.WithError(err).Error("Failed to update user config with push settings")
		h.linebotClient.ReplyMessage(replyToken, "抱歉，設定過程發生錯誤，請稍後再試。")
		return
	}

	// 清理臨時存儲
	h.clearTempDailyWords(userID)
	if tempCourse != "" {
		h.clearTempCourse(userID)
	}

	// 統一的成功訊息處理
	var courseName string
	if finalCourse == "toeic" {
		courseName = "多益"
	} else {
		courseName = "雅思"
	}

	message := fmt.Sprintf("🎉 推播設定完成！\n\n📱 你的推播設定：\n• 課程：%s\n• 每天 %d 個單字\n• 推播時間：%s\n\n🚀 馬上為您推播 %s 單字，下一次會於明天 %s 推播！\n\n現在你可以開始使用翻譯功能！", courseName, dailyWords, pushTime, courseName, pushTime)

	// 設定推播排程並立即推播
	if err := h.setupUserPushSchedule(userID, pushTime, "Asia/Taipei"); err != nil {
		errorMessage := "⚠️ 排程建立失敗，請稍後重新設定或聯絡客服。"
		if replyErr := h.linebotClient.ReplyMessage(replyToken, errorMessage); replyErr != nil {
			h.logger.Error("Failed to send error message: ", replyErr)
		}
		return
	}

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
	h.logger.WithField("userID", userID).Info("Triggering immediate word push")

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

	h.logger.WithField("userID", userID).Info("Successfully triggered immediate word push")
}

// deleteExistingSchedule 刪除現有的用戶排程（如果存在）
func (h *Handler) deleteExistingSchedule(userID string) error {
	scheduleName := fmt.Sprintf("daily-vocab-%s", userID)

	h.logger.WithFields(logrus.Fields{
		"userID":       userID,
		"scheduleName": scheduleName,
	}).Info("Checking for existing schedule")

	// 先檢查排程是否存在
	_, err := h.schedulerClient.GetSchedule(context.TODO(), &scheduler.GetScheduleInput{
		Name:      aws.String(scheduleName),
		GroupName: aws.String("default"),
	})

	if err != nil {
		// 如果排程不存在，直接返回 nil（這是正常情況）
		h.logger.WithField("userID", userID).Info("No existing schedule found")
		return nil
	}

	// 排程存在，刪除它
	h.logger.WithField("userID", userID).Info("Deleting existing schedule")
	_, err = h.schedulerClient.DeleteSchedule(context.TODO(), &scheduler.DeleteScheduleInput{
		Name:      aws.String(scheduleName),
		GroupName: aws.String("default"),
	})

	if err != nil {
		h.logger.WithError(err).Error("Failed to delete existing schedule")
		return fmt.Errorf("failed to delete existing schedule: %w", err)
	}

	h.logger.WithField("userID", userID).Info("Successfully deleted existing schedule")
	return nil
}

// scheduleWordPush 為用戶創建 EventBridge Scheduler 排程
func (h *Handler) scheduleWordPush(userID, pushTime, timezone string) error {
	h.logger.WithFields(logrus.Fields{
		"userID":   userID,
		"pushTime": pushTime,
		"timezone": timezone,
	}).Info("Creating EventBridge schedule for user")

	// 先刪除現有的排程（如果存在）
	if err := h.deleteExistingSchedule(userID); err != nil {
		return fmt.Errorf("failed to delete existing schedule: %w", err)
	}

	// 創建每日 cron 表達式
	scheduleExpression, err := h.createDailyCronExpression(pushTime, timezone)
	if err != nil {
		return fmt.Errorf("failed to create cron expression: %w", err)
	}

	// 準備 Lambda target payload
	payload, err := json.Marshal(map[string]string{
		"userId": userID,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// 創建 schedule
	scheduleName := fmt.Sprintf("daily-vocab-%s", userID)

	h.logger.WithFields(logrus.Fields{
		"scheduleName": scheduleName,
		"expression":   scheduleExpression,
		"targetArn":    h.envVars.vocabularyFunctionArn,
		"roleArn":      h.envVars.schedulerRoleArn,
		"groupName":    "default",
	}).Info("Creating EventBridge schedule")

	scheduleOutput, err := h.schedulerClient.CreateSchedule(context.TODO(), &scheduler.CreateScheduleInput{
		Name:      aws.String(scheduleName),
		GroupName: aws.String("default"),
		FlexibleTimeWindow: &types.FlexibleTimeWindow{
			Mode: types.FlexibleTimeWindowModeOff,
		},
		ScheduleExpression: aws.String(scheduleExpression),
		Target: &types.Target{
			Arn:     aws.String(h.envVars.vocabularyFunctionArn),
			RoleArn: aws.String(h.envVars.schedulerRoleArn),
			Input:   aws.String(string(payload)),
		},
	})
	if err != nil {
		h.logger.WithError(err).Errorf("Failed to create EventBridge schedule: %s", err.Error())
		return fmt.Errorf("failed to create schedule: %w", err)
	}

	h.logger.WithFields(logrus.Fields{
		"scheduleName": scheduleName,
		"userID":       userID,
		"scheduleArn":  aws.ToString(scheduleOutput.ScheduleArn),
	}).Info("Successfully created EventBridge schedule")

	return nil
}

// createDailyCronExpression 創建每日 cron 表達式
func (h *Handler) createDailyCronExpression(pushTime, timezone string) (string, error) {
	// 解析時間 (格式: "HH:MM")
	t, err := time.Parse("15:04", pushTime)
	if err != nil {
		return "", fmt.Errorf("invalid time format: %s", pushTime)
	}

	// 載入時區
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return "", fmt.Errorf("invalid timezone: %s", timezone)
	}

	// 將時間轉換為 UTC（EventBridge Scheduler 使用 UTC）
	now := time.Now().In(loc)
	todayAtPushTime := time.Date(
		now.Year(), now.Month(), now.Day(),
		t.Hour(), t.Minute(), 0, 0, loc,
	)
	utcTime := todayAtPushTime.UTC()

	// 創建 cron 表達式: 分 時 日 月 星期 年
	// 每天在指定時間執行
	cronExpression := fmt.Sprintf("cron(%d %d * * ? *)", utcTime.Minute(), utcTime.Hour())

	h.logger.WithFields(logrus.Fields{
		"originalTime": pushTime,
		"timezone":     timezone,
		"utcTime":      utcTime.Format("15:04"),
		"cronExpr":     cronExpression,
	}).Info("Created daily cron expression")

	return cronExpression, nil
}

// setupUserPushSchedule 設定用戶推播排程並立即推播一次
func (h *Handler) setupUserPushSchedule(userID, pushTime, timezone string) error {
	// 先建立每日推播排程
	if err := h.scheduleWordPush(userID, pushTime, timezone); err != nil {
		h.logger.WithError(err).Error("Failed to create schedule")
		return err
	}

	// 排程建立成功後，立即推播第一次單字
	go h.triggerImmediateWordPush(userID)

	return nil
}
