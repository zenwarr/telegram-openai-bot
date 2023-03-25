package src

import (
	tgapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sashabaranov/go-openai"
	"os"
)

type AppContext struct {
	Config      *Config
	TelegramBot *tgapi.BotAPI
	OpenAI      *openai.Client
	Database    *Database
}

func NewAppContext() (*AppContext, error) {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.json"
	}

	config, err := NewConfig(configPath)
	if err != nil {
		return nil, err
	}

	tg, err := tgapi.NewBotAPI(config.TelegramToken)
	if err != nil {
		return nil, err
	}

	openaiClient := openai.NewClient(config.OpenAIApiKey)

	db, err := NewDatabase("db")
	if err != nil {
		return nil, err
	}

	return &AppContext{
		Config:      config,
		TelegramBot: tg,
		OpenAI:      openaiClient,
		Database:    db,
	}, nil
}
