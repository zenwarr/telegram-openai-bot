package src

import (
	"fmt"
	tgapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func GetDialogId(appContext *AppContext, update *tgapi.Update) string {
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
