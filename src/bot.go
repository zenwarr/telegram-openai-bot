package src

import (
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
	"openai-telegram-bot/src/protos"
	"strings"
	"time"
)

func setBotCommands(appContext *AppContext) {
	setCommands := tgbotapi.NewSetMyCommands(tgbotapi.BotCommand{
		Command:     "help",
		Description: "Usage help",
	}, tgbotapi.BotCommand{
		Command:     "new",
		Description: "Start a new dialog",
	}, tgbotapi.BotCommand{
		Command:     "imagine",
		Description: "Generate image from text",
	})

	_, err := appContext.TelegramBot.Request(setCommands)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set bot commands")
	}
}

func handleUpdate(appContext *AppContext, update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	if !CheckUserAccess(appContext, &update) {
		log.Error().Str("user", GetFormattedSenderName(update.Message)).Msg("Unauthorized user tried to access bot")
		sendNotWantedHere(appContext, update.Message.Chat.ID, update.Message.From.ID, update.Message.MessageID)
		return
	}

	dialogId := GetDialogId(appContext, &update)

	isAnswering := GetDialogEphemeralStatus(dialogId)
	if isAnswering {
		log.Debug().Str("user", GetFormattedSenderName(update.Message)).Msg("Ignored message because model is already answering")
		return
	}

	SetDialogEphemeralStatus(dialogId, true)
	defer SetDialogEphemeralStatus(dialogId, false)

	if handleCommand(appContext, dialogId, update.Message) {
		return
	}

	err := resolveDialogContextLimits(appContext, dialogId, update.Message.Text, update.Message.Chat.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to resolve dialog context limits")
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

	answerMessage(appContext, dialogId, msgText, update.Message)
}

func handleCommand(appContext *AppContext, dialogId string, msg *tgbotapi.Message) bool {
	command := msg.Command()
	if command == "start" || command == "help" {
		sendHello(appContext, msg.Chat.ID)
		return true
	} else if command == "new" {
		err := appContext.Database.DeleteDialog(dialogId)
		if err != nil {
			log.Error().Err(err).Msg("Failed to delete dialog")
		}

		return true
	} else if command == "imagine" {
		generateImage(appContext, msg.CommandArguments(), msg)
		return true
	} else if command != "" {
		sendError(appContext, fmt.Sprintf("Unknown command: %s", command), msg.Chat.ID)
		return true
	}

	return false
}

func generateImage(appContext *AppContext, prompt string, msg *tgbotapi.Message) {
	if !appContext.Config.GenerateImages {
		sendError(appContext, "Image generation is disabled", msg.Chat.ID)
		return
	}

	if prompt == "" {
		sendError(appContext, "Please provide a prompt", msg.Chat.ID)
		return
	}

	endTyping := StartTypingStatus(appContext, msg.Chat.ID)
	defer func() { endTyping <- true }()

	replyUrl, err := Imagine(appContext, prompt)
	if err != nil {
		sendError(appContext, fmt.Sprintf("Failed to generate image: %s", err), msg.Chat.ID)
		return
	}

	replyMsg := tgbotapi.NewPhoto(msg.Chat.ID, tgbotapi.FileURL(replyUrl))
	if appContext.Config.SendReplies {
		replyMsg.ReplyToMessageID = msg.MessageID
	}

	_, err = appContext.TelegramBot.Send(replyMsg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send reply")
	}
}

func answerMessage(appContext *AppContext, dialogId string, msgText string, msg *tgbotapi.Message) {
	err := appContext.Database.AddDialogMessage(dialogId, protos.DialogMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: msgText,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to save dialog message")
		return
	}

	dialogMessages, err := appContext.Database.GetDialog(dialogId)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get dialog messages")
		return
	}

	endTyping := StartTypingStatus(appContext, msg.Chat.ID)
	defer func() { endTyping <- true }()

	replyText := ""
	if appContext.Config.StreamResponse {
		replyText, err = streamingReplyToText(appContext, dialogMessages, msg.Chat.ID, msg.MessageID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get reply")
		}
	} else {
		replyText, err = replyToText(appContext, dialogMessages, msg.Chat.ID, msg.MessageID)
		if err != nil {
			if GetLogicErrorCode(err) == LogicErrorContextLengthExceeded {
				err := appContext.Database.SetDialogState(dialogId, DialogStateContextLimit)
				if err != nil {
					log.Error().Err(err).Msg("Failed to set dialog state")
				}
			}

			log.Error().Err(err).Msg("Failed to get reply")
		}
	}

	err = appContext.Database.AddDialogMessage(dialogId, protos.DialogMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: replyText,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to save dialog message")
	}
}

func resolveDialogContextLimits(appContext *AppContext, dialogId string, userReply string, chatId int64) error {
	dialogState, err := appContext.Database.GetDialogState(dialogId)
	if err != nil {
		return fmt.Errorf("failed to get dialog state: %s", err)
	}

	if dialogState == DialogStateContextLimit {
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
		err = appContext.Database.SetDialogState(dialogId, DialogStateNone)
		if err != nil {
			return fmt.Errorf("failed to delete dialog state: %s", err)
		}
	}

	return nil
}

func summarizeDialog(appContext *AppContext, dialogMessages []protos.DialogMessage) (string, error) {
	firstSummary, err := GetCompleteReply(appContext, []protos.DialogMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: "Summarize this: \n\n" + mergeDialog(dialogMessages[:len(dialogMessages)/2]),
		},
	})
	if err != nil {
		return "", err
	}

	summary, err := GetCompleteReply(appContext, []protos.DialogMessage{
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

func isVoiceMsg(msg *tgbotapi.Message) bool {
	return msg.Voice != nil
}

func getTextFromMsg(appContext *AppContext, msg *tgbotapi.Message) (string, error) {
	if msg.Voice != nil {
		if !appContext.Config.DecodeVoice {
			return "", fmt.Errorf("voice decoding is disabled")
		}

		msgText, err := DecodeVoice(appContext, msg.Voice)
		if err != nil {
			return "", err
		}

		decodedMsg := tgbotapi.NewMessage(msg.Chat.ID, "Decoded: "+msgText)

		_, err = appContext.TelegramBot.Send(decodedMsg)
		if err != nil {
			log.Error().Err(err).Msg("Failed to send decoded message")
		}

		return msgText, nil
	} else if msg.Text != "" {
		return msg.Text, nil
	} else {
		return "", fmt.Errorf("unsupported message type")
	}
}

func replyToText(appContext *AppContext, dialogMessages []protos.DialogMessage, chatID int64, messageID int) (string, error) {
	reply, err := GetCompleteReply(appContext, dialogMessages)
	if err != nil {
		if logicErr, ok := err.(LogicError); ok && logicErr.Code == LogicErrorContextLengthExceeded {
			handleContextLengthExceeded(appContext, chatID, len(dialogMessages))
			return "", err
		}

		return "", err
	}

	msg := tgbotapi.NewMessage(chatID, reply)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true

	if appContext.Config.SendReplies {
		msg.ReplyToMessageID = messageID
	}

	_, err = appContext.TelegramBot.Send(msg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send reply")
	}

	return msg.Text, nil
}

var dialogCloseKeyboard = tgbotapi.NewReplyKeyboard(
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton("Start anew"),
		tgbotapi.NewKeyboardButton("Forget beginning"),
		tgbotapi.NewKeyboardButton("Summarize history"),
	),
)

func handleContextLengthExceeded(appContext *AppContext, chatId int64, messageCount int) {
	msg := tgbotapi.NewMessage(chatId, fmt.Sprintf("‼ Dialog context is too long (%d messages total). Please choose how to continue:", messageCount))
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true
	msg.ReplyMarkup = dialogCloseKeyboard

	_, err := appContext.TelegramBot.Send(msg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send reply")
	}
}

func streamingReplyToText(appContext *AppContext, dialogMessages []protos.DialogMessage, chatId int64, replyTo int) (string, error) {
	replyCh := make(chan string)

	sentMsgId := 0
	completeText := strings.Builder{}
	updateTimer := time.NewTimer(time.Second)
	updatedSinceLastTimer := false

	go func() {
		StreamReply(appContext, dialogMessages, replyCh)
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

func updateMsg(appContext *AppContext, chatId int64, messageId int, text string) {
	edit := tgbotapi.NewEditMessageText(chatId, messageId, text)

	_, err := appContext.TelegramBot.Send(edit)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send reply")
	}
}

func sendInitialMsg(appContext *AppContext, chatId int64, text string, replyTo int) int {
	msg := tgbotapi.NewMessage(chatId, text)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true

	if appContext.Config.SendReplies {
		msg.ReplyToMessageID = replyTo
	}

	sentMsg, err := appContext.TelegramBot.Send(msg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send reply")
	}

	return sentMsg.MessageID
}

func sendHello(appContext *AppContext, chatId int64) {
	helpMsg := appContext.Config.GetMessage("help", "Type anything to start a conversation")

	msg := tgbotapi.NewMessage(chatId, helpMsg)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true

	_, err := appContext.TelegramBot.Send(msg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send hello message")
	}
}

func sendError(appContext *AppContext, message string, chatId int64) {
	msg := tgbotapi.NewMessage(chatId, "‼ "+message)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true

	_, err := appContext.TelegramBot.Send(msg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send error message")
	}
}

func sendNotWantedHere(appContext *AppContext, chatId int64, userId int64, replyTo int) {
	msgText := appContext.Config.GetMessage("not_wanted_here", "")
	if msgText == "" {
		return
	}

	notWantedAlreadySent, err := appContext.Database.GetNotWantedSent(userId)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get not_wanted_sent from db")
		return
	}

	if notWantedAlreadySent {
		return
	}

	msg := tgbotapi.NewMessage(chatId, msgText)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true

	if replyTo != 0 {
		msg.ReplyToMessageID = replyTo
	}

	_, err = appContext.TelegramBot.Send(msg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send not_wanted_here message")
	}

	err = appContext.Database.SetNotWantedSent(userId)
	if err != nil {
		log.Error().Err(err).Msg("Failed to set not_wanted_sent in db")
	}
}
