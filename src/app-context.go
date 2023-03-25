package src

import (
	tgapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
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
	err := godotenv.Load()
	if err != nil {
		return nil, err
	}

	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.json"
	}

	tg, err := tgapi.NewBotAPI(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if err != nil {
		return nil, err
	}

	openaiClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	db, err := NewDatabase("db")
	if err != nil {
		return nil, err
	}

	config, err := NewConfig(configPath)
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
