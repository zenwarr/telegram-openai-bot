package src

import (
	"context"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sashabaranov/go-openai"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
)

func DecodeVoice(appContext *AppContext, voice *tgbotapi.Voice) (string, error) {
	downloaded, err := DownloadVoice(appContext, voice)
	if err != nil {
		return "", err
	}

	req := openai.AudioRequest{
		Model:    openai.Whisper1,
		FilePath: downloaded,
	}

	resp, err := appContext.OpenAI.CreateTranscription(context.Background(), req)
	if err != nil {
		return "", err
	}

	return resp.Text, nil
}

func DownloadVoice(appContext *AppContext, voice *tgbotapi.Voice) (string, error) {
	file, err := appContext.TelegramBot.GetFile(tgbotapi.FileConfig{
		FileID: voice.FileID,
	})
	if err != nil {
		return "", err
	}

	downloadUrl := file.Link(appContext.TelegramBot.Token)

	downloadedFilePath := path.Join(os.TempDir(), file.FileID+".ogg")
	encodedFilePath := downloadedFilePath + ".mp3"

	err = DownloadFile(downloadUrl, downloadedFilePath)
	if err != nil {
		return "", err
	}

	err = EncodeVoice(downloadedFilePath, encodedFilePath)
	if err != nil {
		return "", err
	}

	return encodedFilePath, nil
}

func DownloadFile(url string, filePath string) error {
	exists, err := FileExists(filePath)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	// Create an HTTP client
	httpClient := &http.Client{}

	// Send a GET request to the URL
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Create the file on disk
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Copy the contents of the response body to the file
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func EncodeVoice(input, output string) error {
	exists, err := FileExists(output)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	cmd := exec.Command("ffmpeg", "-i", input, "-vn", "-ar", "44100", "-ac", "2", "-ab", "192k", "-f", "mp3", output)

	return cmd.Run()
}

func FileExists(filePath string) (bool, error) {
	_, err := os.Stat(filePath)

	// Check if the file exists
	if err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, err
	}
}
