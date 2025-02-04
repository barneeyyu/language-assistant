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

type ParserPrompt struct {
	SystemPrompt string `yaml:"system_prompt"`
}

type TranslationResponse struct {
	Translations []Translation `json:"translations"`
}

type Translation struct {
	Word         string   `json:"word"`
	PartOfSpeech string   `json:"partOfSpeech"`
	Meaning      string   `json:"meaning"`
	Example      Example  `json:"example"`
	Synonyms     []string `json:"synonyms"`
}

type Example struct {
	En string `json:"en"`
	Zh string `json:"zh"`
}

type OpenaiAPI interface {
	Translate(inputMsg string) (string, error)
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

func (c *OpenaiClient) Translate(inputMsg string) (string, error) {
	var prompt ParserPrompt
	err := yaml.Unmarshal(translationParserYAML, &prompt)
	if err != nil {
		return "", fmt.Errorf("error parsing prompt yaml: %w", err)
	}

	resp, err := c.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT4,
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
			Temperature: 0.0,
		},
	)
	if err != nil {
		return "", fmt.Errorf("OpenAI API error: %w", err)
	}

	content := resp.Choices[0].Message.Content
	fmt.Printf("Raw OpenAI response:\n%s\n", content)

	fmt.Println("resp from openai: ", resp.Choices[0].Message.Content)
	var translationResponse TranslationResponse
	err = json.Unmarshal([]byte(resp.Choices[0].Message.Content), &translationResponse)
	if err != nil {
		return "", fmt.Errorf("error unmarshalling openai API response: %w", err)
	}

	return translationResponse.String(), nil
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
