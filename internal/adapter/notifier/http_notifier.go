package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/vertex/chat-service/internal/domain"
)

// serviceTokenHeader ต้องตรงกับที่ notification-service อ่าน
const serviceTokenHeader = "X-Service-Token"

// sendTimeout พอร์ตค่าจาก ev-service/internal/adapter/notifier/http_notifier.go
const sendTimeout = 5 * time.Second

type HTTPNotifier struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewHTTPNotifier(baseURL, token string) *HTTPNotifier {
	return &HTTPNotifier{baseURL: baseURL, token: token, client: &http.Client{Timeout: sendTimeout}}
}

func (n *HTTPNotifier) Notify(ctx context.Context, user domain.UserID, title, body, tag, url string) error {
	payload, err := json.Marshal(map[string]string{
		"userId": string(user), "title": title, "body": body, "tag": tag, "url": url,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		n.baseURL+"/api/v1/internal/push/send", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(serviceTokenHeader, n.token)

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("notification-service ตอบ status %d", resp.StatusCode)
	}
	return nil
}
