package utils

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sashabaranov/go-openai"
	"gopkg.in/yaml.v2"
)

//go:embed prompt/translation_parser.yaml
var translationParserYAML []byte

//go:embed prompt/word_generator.yaml
var wordGeneratorYAML []byte

type ParserPrompt struct {
	SystemPrompt string `yaml:"system_prompt"`
}

type TranslationResponse struct {
	Translations []Translation `json:"translations"`
}

type WordGenerationResponse struct {
	Words []Word `json:"words"`
}

type Word struct {
	Word         string   `json:"word"`
	PartOfSpeech string   `json:"partOfSpeech"`
	Meaning      string   `json:"meaning"`
	Example      Example  `json:"example"`
	Synonyms     []string `json:"synonyms"`
	Antonyms     []string `json:"antonyms"`
	Difficulty   string   `json:"difficulty"`
	Category     string   `json:"category"`
}

type Translation struct {
	Word         string   `json:"word"`
	PartOfSpeech string   `json:"partOfSpeech"`
	Meaning      string   `json:"meaning"`
	Example      Example  `json:"example"`
	Synonyms     []string `json:"synonyms"`
	Antonyms     []string `json:"antonyms"`
}

type Example struct {
	En string `json:"en"`
	Zh string `json:"zh"`
}

type OpenaiAPI interface {
	Translate(inputMsg string) (TranslationResponse, error)
	GenerateWord(course string, wordCount int, level int) (WordGenerationResponse, error)
}

type OpenaiClient struct {
	client *openai.Client
}

func NewOpenAIClient(apiKey string, baseUrl string) (OpenaiAPI, error) {
	config := openai.DefaultConfig(apiKey)
	config.BaseURL = baseUrl
	client := openai.NewClientWithConfig(config)
	return &OpenaiClient{
		client: client,
	}, nil
}

func (c *OpenaiClient) Translate(inputMsg string) (TranslationResponse, error) {
	var prompt ParserPrompt
	err := yaml.Unmarshal(translationParserYAML, &prompt)
	if err != nil {
		return TranslationResponse{}, fmt.Errorf("error parsing prompt yaml: %w", err)
	}

	resp, err := c.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT4oMini,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: prompt.SystemPrompt,
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: inputMsg,
				},
			},
			Temperature: 1.0,
		},
	)
	if err != nil {
		return TranslationResponse{}, fmt.Errorf("OpenAI API error: %w", err)
	}

	content := resp.Choices[0].Message.Content

	if !strings.Contains(content, "{") {
		return TranslationResponse{
			Translations: []Translation{
				{
					Word:    inputMsg,
					Meaning: strings.Trim(strings.TrimSpace(content), "\""),
				},
			},
		}, nil
	}
	var translationResponse TranslationResponse
	err = json.Unmarshal([]byte(resp.Choices[0].Message.Content), &translationResponse)
	if err != nil {
		return TranslationResponse{}, fmt.Errorf("error unmarshalling openai API response: %w", err)
	}

	return translationResponse, nil
}

func (c *OpenaiClient) GenerateWord(course string, wordCount int, level int) (WordGenerationResponse, error) {
	var prompt ParserPrompt
	err := yaml.Unmarshal(wordGeneratorYAML, &prompt)
	if err != nil {
		return WordGenerationResponse{}, fmt.Errorf("error parsing word generator prompt yaml: %w", err)
	}

	// Replace template variables in the system prompt
	systemPrompt := strings.ReplaceAll(prompt.SystemPrompt, "{{.Course}}", course)
	systemPrompt = strings.ReplaceAll(systemPrompt, "{{.WordCount}}", fmt.Sprintf("%d", wordCount))
	systemPrompt = strings.ReplaceAll(systemPrompt, "{{.Level}}", fmt.Sprintf("%d", level))

	resp, err := c.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT5,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: systemPrompt,
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: fmt.Sprintf("請生成 %d 個適合 %s 考試 %d 分程度的英文單字", wordCount, course, level),
				},
			},
			Temperature: 1.0,
		},
	)
	if err != nil {
		return WordGenerationResponse{}, fmt.Errorf("OpenAI API error: %w", err)
	}

	content := resp.Choices[0].Message.Content

	var wordResponse WordGenerationResponse
	err = json.Unmarshal([]byte(content), &wordResponse)
	if err != nil {
		return WordGenerationResponse{}, fmt.Errorf("error unmarshalling word generation API response: %w", err)
	}

	return wordResponse, nil
}

func (t Translation) String() string {
	var sb strings.Builder

	// 標題：單字和詞性
	sb.WriteString(fmt.Sprintf("【%s】(%s)\n", t.Word, t.PartOfSpeech))

	// 中文意思
	sb.WriteString(fmt.Sprintf("意思：%s\n", t.Meaning))

	// 例句
	sb.WriteString("例句：\n")
	sb.WriteString(fmt.Sprintf("  %s\n", t.Example.En))
	sb.WriteString(fmt.Sprintf("  %s\n", t.Example.Zh))

	// 同義詞
	if len(t.Synonyms) > 0 {
		sb.WriteString(fmt.Sprintf("同義詞：%s\n", strings.Join(t.Synonyms, ", ")))
	}

	if len(t.Antonyms) > 0 {
		sb.WriteString(fmt.Sprintf("反義詞：%s\n", strings.Join(t.Antonyms, ", ")))
	}

	return sb.String()
}

func (tr TranslationResponse) String() string {
	var sb strings.Builder

	for i, trans := range tr.Translations {
		if i > 0 {
			sb.WriteString("\n-------------------\n")
		}
		sb.WriteString(trans.String())
	}

	return sb.String()
}
