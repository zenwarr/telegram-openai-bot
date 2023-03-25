package src

import (
	"encoding/json"
	"os"
)

type Config struct {
	TelegramToken string `json:"telegram_token"`
	OpenAIApiKey  string `json:"openai_api_key"`
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
