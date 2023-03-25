package main

import (
	"context"
	tgapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sashabaranov/go-openai"
	"log"
	"openai-telegram-bot/src"
	"openai-telegram-bot/src/protos"
)

func main() {
	appContext, err := src.NewAppContext()
	if err != nil {
		log.Fatalf("Failed to initialize app: %s", err)
	}

	log.Printf("Authorized on account %s", appContext.TelegramBot.Self.UserName)

	u := tgapi.NewUpdate(0)
	u.Timeout = 60

	updates := appContext.TelegramBot.GetUpdatesChan(u)

	for update := range updates {
		handleUpdate(appContext, update)
	}
}

func handleUpdate(appContext *src.AppContext, update tgapi.Update) {
	if update.Message == nil {
		return
	}

	msgText := update.Message.Text

	if msgText == "/start" {
		// onStart()
	} else {
		dialogId := src.GetDialogId(appContext, &update)

		err := appContext.Database.AddDialogMessage(dialogId, protos.DialogMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: update.Message.Text,
		})
		if err != nil {
			log.Printf("Failed to save dialog message: %s", err)
			return
		}

		dialogMessages, err := appContext.Database.GetDialog(dialogId)
		if err != nil {
			log.Printf("Failed to get dialog messages: %s", err)
			return
		}

		actionConfig := tgapi.NewChatAction(update.Message.Chat.ID, "typing")
		_, err = appContext.TelegramBot.Send(actionConfig)
		if err != nil {
			log.Printf("Failed to send typing action: %s", err)
		}

		replyMsg, err := replyToText(appContext, dialogMessages, update.Message.Chat.ID, update.Message.MessageID)
		if err != nil {
			log.Printf("Failed to get reply: %s", err)
		}

		err = appContext.Database.AddDialogMessage(dialogId, protos.DialogMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: replyMsg.Text,
		})
		if err != nil {
			log.Printf("Failed to save dialog message: %s", err)
		}

		_, err = appContext.TelegramBot.Send(replyMsg)
		if err != nil {
			log.Printf("Failed to send reply: %s", err)
		}
	}
}

func replyToText(appContext *src.AppContext, dialogMessages []protos.DialogMessage, chatID int64, messageID int) (*tgapi.MessageConfig, error) {
	reply, err := getAIReply(appContext, dialogMessages)
	if err != nil {
		return nil, err
	}

	msg := tgapi.NewMessage(chatID, reply)
	msg.ReplyToMessageID = messageID

	return &msg, nil
}

func getAIReply(appContext *src.AppContext, messages []protos.DialogMessage) (string, error) {
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
