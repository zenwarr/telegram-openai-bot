package src

import (
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog/log"
)

func Run() {
	appContext, err := NewAppContext()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize app")
	}

	log.Info().Msgf("Authorized on account %s", appContext.TelegramBot.Self.UserName)

	setBotCommands(appContext)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := appContext.TelegramBot.GetUpdatesChan(u)

	for update := range updates {
		go handleUpdate(appContext, update)
	}
}
