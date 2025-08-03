package models

type UserConfig struct {
	UserID     string `json:"userId"`
	Course     string `json:"course"`     // "toeic" or "ielts"
	Level      int    `json:"level"`      // 分數
	DailyWords int    `json:"dailyWords"` // 每天推播單字量 (預設10)
	PushTime   string `json:"pushTime"`   // 推播時間 "HH:MM" (預設"08:00")
	Timezone   string `json:"timezone"`   // 時區 (預設"Asia/Taipei")
	UpdatedAt  string `json:"updatedAt"`  // ISO timestamp
}
