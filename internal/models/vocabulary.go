package models

import (
	"fmt"
	"strings"
)

type UserVocabulary struct {
	UserID    string       `json:"userId"`
	Date      string       `json:"date"` // YYYY-MM-DD
	Words     []WordRecord `json:"words"`
	UpdatedAt string       `json:"updatedAt"` // ISO timestamp
}

type WordRecord struct {
	Word         string `json:"word"`
	PartOfSpeech string `json:"partOfSpeech"`
	Translation  string `json:"translation"`
	Sentence     string `json:"sentence"`
	Timestamp    string `json:"timestamp"` // ISO timestamp
}

func FormatWordRecords(records interface{}) string {
	var sb strings.Builder
	switch v := records.(type) {
	case WordRecord:
		sb.WriteString(fmt.Sprintf("【%s】(%s)\n", v.Word, v.PartOfSpeech))
		sb.WriteString(fmt.Sprintf("翻譯：%s\n", v.Translation))
		sb.WriteString("例句：\n")
		sb.WriteString(fmt.Sprintf("  %s\n", v.Sentence))
	case []WordRecord:
		for i, w := range v {
			if i > 0 {
				sb.WriteString("\n-------------------\n")
			}
			sb.WriteString(FormatWordRecords(w))
		}
	}
	return sb.String()
}

type UserConfig struct {
	UserID    string `json:"userId"`
	Course    string `json:"course"`    // "toeic" or "ielts"
	Level     int    `json:"level"`     // 分數
	UpdatedAt string `json:"updatedAt"` // ISO timestamp
}
