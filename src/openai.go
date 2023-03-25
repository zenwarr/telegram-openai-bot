package src

import (
	"context"
	"github.com/sashabaranov/go-openai"
	"log"
	"openai-telegram-bot/src/protos"
)

func GetCompleteReply(appContext *AppContext, messages []protos.DialogMessage) (string, error) {
	openaiMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		openaiMessages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	resp, err := appContext.OpenAI.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:    openai.GPT3Dot5Turbo0301,
			Messages: openaiMessages,
		},
	)

	if err != nil {
		log.Printf("Failed to get OpenAI reply: %s", err)
		return "", err
	}

	return resp.Choices[0].Message.Content, nil
}
