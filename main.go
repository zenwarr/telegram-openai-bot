package main

import (
	tgapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sashabaranov/go-openai"
	"log"
	"openai-telegram-bot/src"
	"openai-telegram-bot/src/protos"
	"strings"
	"time"
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

		replyText := ""
		if appContext.Config.StreamResponse {
			replyText, err = streamingReplyToText(appContext, dialogMessages, update.Message.Chat.ID, update.Message.MessageID)
			if err != nil {
				log.Printf("Failed to get reply: %s", err)
			}
		} else {
			replyText, err = replyToText(appContext, dialogMessages, update.Message.Chat.ID, update.Message.MessageID)
			if err != nil {
				log.Printf("Failed to get reply: %s", err)
			}
		}

		err = appContext.Database.AddDialogMessage(dialogId, protos.DialogMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: replyText,
		})
		if err != nil {
			log.Printf("Failed to save dialog message: %s", err)
		}
	}
}

func replyToText(appContext *src.AppContext, dialogMessages []protos.DialogMessage, chatID int64, messageID int) (string, error) {
	reply, err := src.GetCompleteReply(appContext, dialogMessages)
	if err != nil {
		return "", err
	}

	msg := tgapi.NewMessage(chatID, reply)
	msg.ParseMode = "Markdown"
	msg.ReplyToMessageID = messageID

	_, err = appContext.TelegramBot.Send(msg)
	if err != nil {
		log.Printf("Failed to send reply: %s", err)
	}

	return msg.Text, nil
}

func streamingReplyToText(appContext *src.AppContext, dialogMessages []protos.DialogMessage, chatId int64, replyTo int) (string, error) {
	replyCh := make(chan string)

	sentMsgId := 0
	completeText := strings.Builder{}
	updateTimer := time.NewTimer(time.Second)
	updatedSinceLastTimer := false

	go func() {
		src.StreamReply(appContext, dialogMessages, replyCh)
	}()

loop:
	for {
		select {
		case delta, ok := <-replyCh:
			if !ok {
				if sentMsgId == 0 {
					sendInitialMsg(appContext, chatId, completeText.String(), replyTo)
				} else {
					updateMsg(appContext, chatId, sentMsgId, completeText.String())
				}

				break loop
			}

			if delta == "" {
				continue
			}

			completeText.WriteString(delta)
			updatedSinceLastTimer = true

		case <-updateTimer.C:
			if !updatedSinceLastTimer {
				continue
			}

			if sentMsgId == 0 {
				sentMsgId = sendInitialMsg(appContext, chatId, completeText.String(), replyTo)
			} else {
				updateMsg(appContext, chatId, sentMsgId, completeText.String())
			}

			updatedSinceLastTimer = false
			updateTimer.Reset(time.Second)
		}
	}

	return completeText.String(), nil
}

func updateMsg(appContext *src.AppContext, chatId int64, messageId int, text string) {
	edit := tgapi.NewEditMessageText(chatId, messageId, text)

	_, err := appContext.TelegramBot.Send(edit)
	if err != nil {
		log.Printf("Failed to edit reply: %s", err)
	}
}

func sendInitialMsg(appContext *src.AppContext, chatId int64, text string, replyTo int) int {
	msg := tgapi.NewMessage(chatId, text)
	msg.ParseMode = "Markdown"
	msg.ReplyToMessageID = replyTo

	sentMsg, err := appContext.TelegramBot.Send(msg)
	if err != nil {
		log.Printf("Failed to send reply: %s", err)
	}

	return sentMsg.MessageID
}
