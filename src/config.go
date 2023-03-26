package src

import (
	"encoding/json"
	"os"
	"time"
)

const DialogContextTrackingModeNone = "none"
const DialogContextTrackingModeUser = "user"
const DialogContextTrackingModeChat = "chat"

type Config struct {
	TelegramToken string `json:"telegram_token"`
	OpenAIApiKey  string `json:"openai_api_key"`

	Users []string `json:"users"`

	DialogContextTrackingMode string `json:"dialog_context_tracking_mode"`
	DialogContextExpire       string `json:"dialog_context_expire"`
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

func (config *Config) GetMessage(name string, def string) string {
	if msg, ok := config.Messages[name]; ok {
		return msg
	}

	return def
}

func (config *Config) GetDialogContextExpireDuration() time.Duration {
	value := config.DialogContextExpire
	if value == "" {
		return 0
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0
	}

	return duration.Abs()
}
