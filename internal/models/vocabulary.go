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
		// 單個單字格式化（不包含標題）
		sb.WriteString(fmt.Sprintf("【%s】(%s)\n", v.Word, v.PartOfSpeech))
		sb.WriteString(fmt.Sprintf("翻譯：%s\n", v.Translation))
		sb.WriteString("例句：\n")
		sb.WriteString(fmt.Sprintf("  %s\n", v.Sentence))
	case []WordRecord:
		// 多個單字格式化（包含標題）
		if len(v) == 0 {
			return "今天還沒有學習任何單字喔！"
		}

		sb.WriteString("【本日單字回顧】📚\n\n")
		for i, w := range v {
			if i > 0 {
				sb.WriteString("\n-------------------\n")
			}
			// 直接格式化單字內容，不要再調用 FormatWordRecords
			sb.WriteString(fmt.Sprintf("%s (%s)\n", w.Word, w.PartOfSpeech))
			sb.WriteString(fmt.Sprintf("翻譯：%s\n", w.Translation))
			sb.WriteString("例句：\n")
			sb.WriteString(fmt.Sprintf("  %s\n", w.Sentence))
		}
	}
	return sb.String()
}
