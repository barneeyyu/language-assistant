package utils

import (
	"encoding/json"
	"testing"
)

func TestTranslationResponseParsing(t *testing.T) {
	// Test case 1: 基本的單一翻譯
	t.Run("Single translation", func(t *testing.T) {
		jsonStr := `{
			"translations": [
				{
					"word": "book",
					"partOfSpeech": "n.",
					"meaning": "書本、書籍",
					"example": {
						"en": "I bought a new book yesterday.",
						"zh": "我昨天買了一本新書。"
					},
					"synonyms": ["novel", "text"]
				}
			]
		}`

		var resp TranslationResponse
		err := json.Unmarshal([]byte(jsonStr), &resp)
		if err != nil {
			t.Errorf("Failed to parse JSON: %v", err)
		}

		// 驗證解析結果
		if len(resp.Translations) != 1 {
			t.Errorf("Expected 1 translation, got %d", len(resp.Translations))
		}

		trans := resp.Translations[0]
		if trans.Word != "book" {
			t.Errorf("Expected word 'book', got '%s'", trans.Word)
		}
	})

	// Test case 2: 多個翻譯
	t.Run("Multiple translations", func(t *testing.T) {
		jsonStr := `{
			"translations": [
				{
					"word": "book",
					"partOfSpeech": "n.",
					"meaning": "書本、書籍",
					"example": {
						"en": "I bought a new book yesterday.",
						"zh": "我昨天買了一本新書。"
					},
					"synonyms": ["novel", "text"]
				},
				{
					"word": "book",
					"partOfSpeech": "v.",
					"meaning": "預訂、預約",
					"example": {
						"en": "I need to book a table for dinner.",
						"zh": "我需要預訂一張晚餐的桌子。"
					},
					"synonyms": ["reserve"]
				}
			]
		}`

		var resp TranslationResponse
		err := json.Unmarshal([]byte(jsonStr), &resp)
		if err != nil {
			t.Errorf("Failed to parse JSON: %v", err)
		}

		// 驗證解析結果
		if len(resp.Translations) != 2 {
			t.Errorf("Expected 2 translations, got %d", len(resp.Translations))
		}
	})

	// Test case 3: 測試 String() 方法的輸出格式
	t.Run("String format", func(t *testing.T) {
		trans := Translation{
			Word:         "book",
			PartOfSpeech: "n.",
			Meaning:      "書本、書籍",
			Example: Example{
				En: "I bought a new book yesterday.",
				Zh: "我昨天買了一本新書。",
			},
			Synonyms: []string{"novel", "text"},
		}

		expected := "【book】(n.)\n" +
			"意思：書本、書籍\n" +
			"例句：\n" +
			"  I bought a new book yesterday.\n" +
			"  我昨天買了一本新書。\n" +
			"同義詞：novel, text\n"

		if trans.String() != expected {
			t.Errorf("String format mismatch.\nExpected:\n%s\nGot:\n%s", expected, trans.String())
		}
	})
}
