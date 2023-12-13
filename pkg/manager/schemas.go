package manager

import "github.com/sashabaranov/go-openai"

type AskChatGPT struct {
	Client   *openai.Client
	Model    string
	Contents []string
	AnswerCh chan string
	DoneCh   chan string
}

type ChatGPTParam struct {
	AuthToken string
	BaseURL   string
	Proxy     string
	Model     string
	Prompt     string
}
