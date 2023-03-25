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

	Users []string `json:"users"`

	DialogContextTrackingMode string `json:"dialog_context_tracking_mode"`
	StreamResponse            bool   `json:"stream_response"`
	SendReplies               bool   `json:"send_replies"`

	DecodeVoice bool `json:"decode_voice"`
	AnswerVoice bool `json:"answer_voice"`

	Messages map[string]string `json:"messages"`
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
