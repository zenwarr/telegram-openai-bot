package src

import (
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func CheckUserAccess(appContext *AppContext, update *tgbotapi.Update) bool {
	allowedUsers := appContext.Config.Users
	if len(allowedUsers) == 0 {
		return true
	}

	userId := update.Message.From.ID
	userName := update.Message.From.UserName

	for _, allowedUser := range allowedUsers {
		if (userName != "" && allowedUser == userName) || allowedUser == fmt.Sprintf("%d", userId) {
			return true
		}
	}

	return false
}
