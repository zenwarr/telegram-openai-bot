package src

import (
	"context"
	"errors"
	"github.com/sashabaranov/go-openai"
	"io"
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
			Model:    openai.GPT3Dot5Turbo,
			Messages: openaiMessages,
		},
	)

	if err != nil {
		if getOpenAIErrorCode(err) == "context_length_exceeded" {
			return "", LogicError{
				Code:    LogicErrorContextLengthExceeded,
				Message: "Context length exceeded",
			}
		}

		return "", err
	}

	return resp.Choices[0].Message.Content, nil
}

func StreamReply(appContext *AppContext, messages []protos.DialogMessage, replyCh chan string) error {
	openaiMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		openaiMessages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	req := openai.ChatCompletionRequest{
		Model:    openai.GPT3Dot5Turbo0301,
		Messages: openaiMessages,
		Stream:   true,
	}

	stream, err := appContext.OpenAI.CreateChatCompletionStream(context.Background(), req)
	if err != nil {
		return err
	}

	defer stream.Close()
	defer close(replyCh)

	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return err
		}

		replyCh <- response.Choices[0].Delta.Content
	}

	return nil
}

func Imagine(appContext *AppContext, prompt string) (string, error) {
	prompt, size := parsePrompt(prompt)

	reqUrl := openai.ImageRequest{
		Prompt:         prompt,
		Size:           size,
		ResponseFormat: openai.CreateImageResponseFormatURL,
		N:              1,
	}

	respUrl, err := appContext.OpenAI.CreateImage(context.Background(), reqUrl)
	if err != nil {
		return "", err
	}

	return respUrl.Data[0].URL, nil
}

// you can specify generated image size by mentioning a pattern like `@mid` in the prompt
// so we need to check presence of this pattern and remove it from the prompt
func parsePrompt(prompt string) (string, string) {
	size := openai.CreateImageSize256x256

	if len(prompt) > 4 && prompt[len(prompt)-4:] == "@mid" {
		size = openai.CreateImageSize512x512
		prompt = prompt[:len(prompt)-4]
	} else if len(prompt) > 5 && prompt[len(prompt)-5:] == "@high" {
		size = openai.CreateImageSize1024x1024
		prompt = prompt[:len(prompt)-5]
	}

	return prompt, size
}
