package src

import (
	"fmt"
	tgapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func GetFormattedUserName(userName string, userId int64) string {
	if userName != "" {
		return fmt.Sprintf("@%s (#%d)", userName, userId)
	} else {
		return fmt.Sprintf("#%d", userId)
	}
}

func GetFormattedSenderName(msg *tgapi.Message) string {
	return GetFormattedUserName(msg.From.UserName, msg.From.ID)
}
