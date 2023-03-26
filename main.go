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
		sendNotWantedHere(appContext, update.Message.Chat.ID, update.Message.From.ID, update.Message.MessageID)
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

	dialogId := src.GetDialogId(appContext, &update)

	err := resolveDialogContextLimits(appContext, dialogId, update.Message.Text, update.Message.Chat.ID)
	if err != nil {
		log.Printf("Failed to resolve dialog context limits: %s", err)
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

	endTyping := src.StartTypingStatus(appContext, update.Message.Chat.ID)
	defer func() { endTyping <- true }()

	replyText := ""
	if appContext.Config.StreamResponse {
		replyText, err = streamingReplyToText(appContext, dialogMessages, update.Message.Chat.ID, update.Message.MessageID)
		if err != nil {
			log.Printf("Failed to get reply: %s", err)
		}
	} else {
		replyText, err = replyToText(appContext, dialogMessages, update.Message.Chat.ID, update.Message.MessageID)
		if err != nil {
			if src.GetLogicErrorCode(err) == src.LogicErrorContextLengthExceeded {
				err := appContext.Database.SetDialogState(dialogId, src.DialogStateContextLimit)
				if err != nil {
					log.Printf("Failed to set dialog state: %s", err)
				}
			}

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

func resolveDialogContextLimits(appContext *src.AppContext, dialogId string, userReply string, chatId int64) error {
	dialogState, err := appContext.Database.GetDialogState(dialogId)
	if err != nil {
		return fmt.Errorf("failed to get dialog state: %s", err)
	}

	if dialogState == src.DialogStateContextLimit {
		if userReply == "Start anew" {
			// delete this dialog
			err := appContext.Database.DeleteDialog(dialogId)
			if err != nil {
				return fmt.Errorf("failed to delete dialog: %s", err)
			}
		} else if userReply == "Forget beginning" {
			err := appContext.Database.DecimateDialog(dialogId)
			if err != nil {
				return fmt.Errorf("failed to decimate dialog: %s", err)
			}
		} else if userReply == "Summarize history" {
			dialogMessages, err := appContext.Database.GetDialog(dialogId)
			if err != nil {
				return fmt.Errorf("failed to get dialog messages: %s", err)
			}

			summary, err := summarizeDialog(appContext, dialogMessages)
			if err != nil {
				return fmt.Errorf("failed to summarize dialog: %s", err)
			}

			err = appContext.Database.ReplaceDialog(dialogId, &protos.DialogMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: summary,
			})
			if err != nil {
				return err
			}

			return nil
		} else {
			sendError(appContext, fmt.Sprintf("Unknown dialog state reply: %s", userReply), chatId)
			return fmt.Errorf("unknown dialog state reply: %s", userReply)
		}

		// delete dialog state
		err = appContext.Database.SetDialogState(dialogId, src.DialogStateNone)
		if err != nil {
			return fmt.Errorf("failed to delete dialog state: %s", err)
		}
	}

	return nil
}

func summarizeDialog(appContext *src.AppContext, dialogMessages []protos.DialogMessage) (string, error) {
	firstSummary, err := src.GetCompleteReply(appContext, []protos.DialogMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: "Summarize this: \n\n" + mergeDialog(dialogMessages[:len(dialogMessages)/2]),
		},
	})
	if err != nil {
		return "", err
	}

	summary, err := src.GetCompleteReply(appContext, []protos.DialogMessage{
		{
			Role: openai.ChatMessageRoleUser,
			Content: fmt.Sprintf(
				"Text after #PREV# and #PREV# is a summary of previous dialog with the assistent. Summarize the dialog that continues with messages between #CONT# and #CONT#: \n\n#PREV#%s#PREV\n\n#CONT#%s#CONT#",
				firstSummary,
				mergeDialog(dialogMessages[len(dialogMessages)/2:]),
			),
		},
	})
	if err != nil {
		return "", err
	}

	summary = strings.ReplaceAll(summary, "#CONT#", "")
	summary = strings.ReplaceAll(summary, "#PREV#", "")

	summary = fmt.Sprintf("This is a summary of previous dialog messages: \n\n%s", summary)

	return summary, err
}

func mergeDialog(dialogMessages []protos.DialogMessage) string {
	builder := strings.Builder{}

	for _, msg := range dialogMessages {
		if msg.Role == openai.ChatMessageRoleUser {
			builder.WriteString("User: " + msg.Content + "#END#")
		} else {
			builder.WriteString("Assistant: " + msg.Content + "#END#")
		}
	}

	return builder.String()
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
		if logicErr, ok := err.(src.LogicError); ok && logicErr.Code == src.LogicErrorContextLengthExceeded {
			handleContextLengthExceeded(appContext, chatID, len(dialogMessages))
			return "", err
		}

		return "", err
	}

	msg := tgapi.NewMessage(chatID, reply)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true

	if appContext.Config.SendReplies {
		msg.ReplyToMessageID = messageID
	}

	_, err = appContext.TelegramBot.Send(msg)
	if err != nil {
		log.Printf("Failed to send reply: %s", err)
	}

	return msg.Text, nil
}

var dialogCloseKeyboard = tgapi.NewReplyKeyboard(
	tgapi.NewKeyboardButtonRow(
		tgapi.NewKeyboardButton("Start anew"),
		tgapi.NewKeyboardButton("Forget beginning"),
		tgapi.NewKeyboardButton("Summarize history"),
	),
)

func handleContextLengthExceeded(appContext *src.AppContext, chatId int64, messageCount int) {
	msg := tgapi.NewMessage(chatId, fmt.Sprintf("‼ Dialog context is too long (%d messages total). Please choose how to continue:", messageCount))
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true
	msg.ReplyMarkup = dialogCloseKeyboard

	_, err := appContext.TelegramBot.Send(msg)
	if err != nil {
		log.Printf("Failed to send reply: %s", err)
	}
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
	msg.DisableWebPagePreview = true

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
	helpMsg := appContext.Config.GetMessage("help", "Type anything to start a conversation")

	msg := tgapi.NewMessage(chatId, helpMsg)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true

	_, err := appContext.TelegramBot.Send(msg)
	if err != nil {
		log.Printf("Failed to send hello message: %s", err)
	}
}

func sendError(appContext *src.AppContext, message string, chatId int64) {
	msg := tgapi.NewMessage(chatId, "‼ "+message)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true

	_, err := appContext.TelegramBot.Send(msg)
	if err != nil {
		log.Printf("Failed to send error message: %s", err)
	}
}

func sendNotWantedHere(appContext *src.AppContext, chatId int64, userId int64, replyTo int) {
	msgText := appContext.Config.GetMessage("not_wanted_here", "")
	if msgText == "" {
		return
	}

	notWantedAlreadySent, err := appContext.Database.GetNotWantedSent(userId)
	if err != nil {
		log.Printf("Failed to get not_wanted_sent from db: %s", err)
		return
	}

	if notWantedAlreadySent {
		return
	}

	msg := tgapi.NewMessage(chatId, msgText)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true

	if replyTo != 0 {
		msg.ReplyToMessageID = replyTo
	}

	_, err = appContext.TelegramBot.Send(msg)
	if err != nil {
		log.Printf("Failed to send not_wanted_here message: %s", err)
	}

	err = appContext.Database.SetNotWantedSent(userId)
	if err != nil {
		log.Printf("Failed to set not_wanted_sent in db: %s", err)
	}
}
