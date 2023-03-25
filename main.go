package main

import (
	"fmt"
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

	if !src.CheckUserAccess(appContext, &update) {
		log.Printf("Unauthorized user %s tried to access bot", src.GetFormattedSenderName(update.Message))
		return
	}

	command := update.Message.Command()
	if command == "start" || command == "help" {
		sendHello(appContext, update.Message.Chat.ID)
		return
	} else if command != "" {
		sendError(appContext, fmt.Sprintf("Unknown command: %s", command), update.Message.Chat.ID)
		return
	}

	msgText, err := getTextFromMsg(appContext, update.Message)
	if err != nil {
		sendError(appContext, fmt.Sprintf("Failed to get text from message: %s", err), update.Message.Chat.ID)
		return
	}

	if isVoiceMsg(update.Message) && !appContext.Config.AnswerVoice {
		return
	}

	dialogId := src.GetDialogId(appContext, &update)

	err = appContext.Database.AddDialogMessage(dialogId, protos.DialogMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: msgText,
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

func isVoiceMsg(msg *tgapi.Message) bool {
	return msg.Voice != nil
}

func getTextFromMsg(appContext *src.AppContext, msg *tgapi.Message) (string, error) {
	if msg.Voice != nil {
		if !appContext.Config.DecodeVoice {
			return "", fmt.Errorf("voice decoding is disabled")
		}

		msgText, err := src.DecodeVoice(appContext, msg.Voice)
		if err != nil {
			return "", err
		}

		decodedMsg := tgapi.NewMessage(msg.Chat.ID, "Decoded: "+msgText)

		_, err = appContext.TelegramBot.Send(decodedMsg)
		if err != nil {
			log.Printf("Failed to send decoded message: %s", err)
		}

		return msgText, nil
	} else if msg.Text != "" {
		return msg.Text, nil
	} else {
		return "", fmt.Errorf("unsupported message type")
	}
}

func replyToText(appContext *src.AppContext, dialogMessages []protos.DialogMessage, chatID int64, messageID int) (string, error) {
	reply, err := src.GetCompleteReply(appContext, dialogMessages)
	if err != nil {
		return "", err
	}

	msg := tgapi.NewMessage(chatID, reply)
	msg.ParseMode = "Markdown"

	if appContext.Config.SendReplies {
		msg.ReplyToMessageID = messageID
	}

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

	if appContext.Config.SendReplies {
		msg.ReplyToMessageID = replyTo
	}

	sentMsg, err := appContext.TelegramBot.Send(msg)
	if err != nil {
		log.Printf("Failed to send reply: %s", err)
	}

	return sentMsg.MessageID
}

func sendHello(appContext *src.AppContext, chatId int64) {
	helpMsg := appContext.Config.Messages["help"]
	if helpMsg == "" {
		helpMsg = "Type anything to start a conversation."
	}

	msg := tgapi.NewMessage(chatId, helpMsg)
	msg.ParseMode = "Markdown"

	_, err := appContext.TelegramBot.Send(msg)
	if err != nil {
		log.Printf("Failed to send hello message: %s", err)
	}
}

func sendError(appContext *src.AppContext, message string, chatId int64) {
	msg := tgapi.NewMessage(chatId, "‼ "+message)
	msg.ParseMode = "Markdown"

	_, err := appContext.TelegramBot.Send(msg)
	if err != nil {
		log.Printf("Failed to send error message: %s", err)
	}
}
