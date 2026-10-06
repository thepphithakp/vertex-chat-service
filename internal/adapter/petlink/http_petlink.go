package petlink

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/vertex/chat-service/internal/domain"
)

const serviceTokenHeader = "X-Service-Token"
const requestTimeout = 5 * time.Second

// HTTPPetLink เรียก pet-service เพื่อตรวจว่าสองคนนี้มีสัตว์เลี้ยงร่วมกันไหม
// (เจ้าของ/ผู้ดูแล) — เป็นคำถามเดียวที่ chat-service เองตอบไม่ได้ เพราะไม่มี
// สิทธิ์เห็นข้อมูล pet เลย (คนละ schema คนละ DB role)
type HTTPPetLink struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewHTTPPetLink(baseURL, token string) *HTTPPetLink {
	return &HTTPPetLink{baseURL: baseURL, token: token, client: &http.Client{Timeout: requestTimeout}}
}

type linkResponse struct {
	Linked bool `json:"linked"`
}

func (p *HTTPPetLink) SharePet(ctx context.Context, a, b domain.UserID, petID uuid.UUID) (bool, error) {
	u := fmt.Sprintf("%s/internal/v1/pets/%s/link?userA=%s&userB=%s",
		p.baseURL, petID.String(), url.QueryEscape(string(a)), url.QueryEscape(string(b)))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set(serviceTokenHeader, p.token)

	resp, err := p.client.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("pet-service ตอบ status %d", resp.StatusCode)
	}

	var out linkResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, err
	}
	return out.Linked, nil
}
