package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"backup-service/internal/config"
)

type Telegram struct {
	enabled        bool
	successEnabled bool
	botToken       string
	chatID         string
	client         *http.Client
}

func NewTelegram(cfg *config.Config) *Telegram {
	return &Telegram{
		enabled:        cfg.Telegram.Enabled,
		successEnabled: cfg.TelegramSuccessEnabled(),
		botToken:       cfg.Telegram.BotToken,
		chatID:         cfg.Telegram.ChatID,
		client:         &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *Telegram) Success(ctx context.Context, text string) error {
	if !t.enabled || !t.successEnabled {
		return nil
	}
	return t.send(ctx, text)
}

func (t *Telegram) Failure(ctx context.Context, text string) error {
	if !t.enabled {
		return nil
	}
	return t.send(ctx, text)
}

func (t *Telegram) send(ctx context.Context, text string) error {
	body, err := json.Marshal(map[string]string{
		"chat_id": t.chatID,
		"text":    text,
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram returned status %s", resp.Status)
	}
	return nil
}
