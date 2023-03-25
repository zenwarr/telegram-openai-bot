package src

import (
	"encoding/json"
	"os"
)

const DialogContextTrackingModeNone = "none"
const DialogContextTrackingModeUser = "user"
const DialogContextTrackingModeChat = "chat"

type Config struct {
	TelegramToken string `json:"telegram_token"`
	OpenAIApiKey  string `json:"openai_api_key"`

	DialogContextTrackingMode string `json:"dialog_context_tracking_mode"`
}

func NewConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config Config
	err = json.NewDecoder(file).Decode(&config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}
