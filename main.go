package main

import (
	"context"
	tgapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	"github.com/sashabaranov/go-openai"
	"log"
	"openai-telegram-bot/src"
	"openai-telegram-bot/src/protos"
	"os"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	tg, err := tgapi.NewBotAPI(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if err != nil {
		log.Fatal(err)
	}

	openaiClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	db, err := src.NewDatabase("db")
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Authorized on account %s", tg.Self.UserName)

	u := tgapi.NewUpdate(0)
	u.Timeout = 60

	updates := tg.GetUpdatesChan(u)

	for update := range updates {
		handleUpdate(db, openaiClient, tg, update)
	}
}

func handleUpdate(db *src.Database, openaiClient *openai.Client, bot *tgapi.BotAPI, update tgapi.Update) {
	if update.Message == nil {
		return
	}

	msgText := update.Message.Text

	if msgText == "/start" {
		// onStart()
	} else {
		err := db.AddDialogMessage(update.Message.Chat.ID, protos.DialogMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: update.Message.Text,
		})
		if err != nil {
			log.Println(err)
			return
		}

		dialogMessages, err := db.GetDialog(update.Message.Chat.ID)
		if err != nil {
			log.Println(err)
			return
		}

		actionConfig := tgapi.NewChatAction(update.Message.Chat.ID, "typing")
		_, err = bot.Send(actionConfig)
		if err != nil {
			log.Println(err)
		}

		replyMsg, err := replyToText(openaiClient, dialogMessages, update.Message.Chat.ID, update.Message.MessageID)
		if err != nil {
			log.Println(err)
		}

		err = db.AddDialogMessage(update.Message.Chat.ID, protos.DialogMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: replyMsg.Text,
		})
		if err != nil {
			log.Println(err) // but still send the message
		}

		bot.Send(replyMsg)
	}
}

func replyToText(openaiClient *openai.Client, dialogMessages []protos.DialogMessage, chatID int64, messageID int) (*tgapi.MessageConfig, error) {
	reply, err := getAIReply(openaiClient, dialogMessages)
	if err != nil {
		return nil, err
	}

	msg := tgapi.NewMessage(chatID, reply)
	msg.ReplyToMessageID = messageID

	return &msg, nil
}

func getAIReply(openaiClient *openai.Client, messages []protos.DialogMessage) (string, error) {
	openaiMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		openaiMessages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	resp, err := openaiClient.CreateChatCompletion(
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
