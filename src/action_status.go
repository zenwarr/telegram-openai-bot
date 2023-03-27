package src

import (
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog/log"
	"time"
)

func StatusUpdate(ch chan interface{}, f func()) {
	ticker := time.NewTicker(5 * time.Second)

	f()

	for {
		select {
		case <-ticker.C:
			f()
		case <-ch:
			ticker.Stop()
			return
		}
	}
}

func StartTypingStatus(appContext *AppContext, chatId int64) chan interface{} {
	ch := make(chan interface{})

	go StatusUpdate(ch, func() {
		actionConfig := tgbotapi.NewChatAction(chatId, "typing")
		_, err := appContext.TelegramBot.Send(actionConfig)
		if err != nil {
			log.Error().Err(err).Msg("Failed to send typing action")
		}
	})

	return ch
}
