package src

import (
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func GetDialogId(appContext *AppContext, update *tgbotapi.Update) string {
	mode := appContext.Config.DialogContextTrackingMode
	if mode == DialogContextTrackingModeNone {
		return fmt.Sprintf("msg:%d", update.Message.MessageID)
	} else if mode == DialogContextTrackingModeChat {
		return fmt.Sprintf("chat:%d", update.Message.Chat.ID)
	} else if mode == DialogContextTrackingModeUser {
		return fmt.Sprintf("user:%d", update.Message.From.ID)
	} else {
		return fmt.Sprintf("chat:%d", update.Message.Chat.ID)
	}
}
