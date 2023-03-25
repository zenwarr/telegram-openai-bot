package src

import (
	tgapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
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
		actionConfig := tgapi.NewChatAction(chatId, "typing")
		_, err := appContext.TelegramBot.Send(actionConfig)
		if err != nil {
			log.Printf("Failed to send typing action: %s", err)
		}
	})

	return ch
}
