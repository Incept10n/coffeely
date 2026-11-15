package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Incept10n/coffeely/backend/internal/config"
)

// Yandex — OAuth2-клиент Yandex ID. URLs берутся из конфига, чтобы тесты
// гоняли флоу против mock-сервера.
type Yandex struct {
	cfg   config.Config
	http  *http.Client
	state struct {
		sync.Mutex
		m map[string]time.Time
	}
}

func NewYandex(cfg config.Config) *Yandex {
	y := &Yandex{cfg: cfg, http: &http.Client{Timeout: 10 * time.Second}}
	y.state.m = make(map[string]time.Time)
	return y
}

// StartURL строит ссылку, на которую надо отправить гостя.
func (y *Yandex) StartURL() (string, error) {
	state, err := randState()
	if err != nil {
		return "", err
	}
	y.state.Lock()
	y.state.m[state] = time.Now().Add(10 * time.Minute)
	y.state.Unlock()

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", y.cfg.YandexClientID)
	q.Set("redirect_uri", y.cfg.YandexRedirectURL)
	q.Set("state", state)
	return y.cfg.YandexAuthURL + "?" + q.Encode(), nil
}

// Callback меняет code на токен и тянет профиль (email, yandex_id).
func (y *Yandex) Callback(ctx context.Context, code, state string) (yandexID, email string, err error) {
	if !y.takeState(state) {
		return "", "", fmt.Errorf("state неизвестен или устарел")
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", y.cfg.YandexRedirectURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, y.cfg.YandexTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.SetBasicAuth(url.QueryEscape(y.cfg.YandexClientID), url.QueryEscape(y.cfg.YandexClientSecret))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := y.http.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("yandex token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", "", fmt.Errorf("yandex token: статус %d: %s", resp.StatusCode, body)
	}
	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil || tokenResp.AccessToken == "" {
		return "", "", fmt.Errorf("yandex token: ответ без access_token: %w", err)
	}

	infoReq, err := http.NewRequestWithContext(ctx, http.MethodGet, y.cfg.YandexInfoURL, nil)
	if err != nil {
		return "", "", err
	}
	infoReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	infoResp, err := y.http.Do(infoReq)
	if err != nil {
		return "", "", fmt.Errorf("yandex info: %w", err)
	}
	defer infoResp.Body.Close()
	if infoResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(infoResp.Body, 4096))
		return "", "", fmt.Errorf("yandex info: статус %d: %s", infoResp.StatusCode, body)
	}
	var info struct {
		ID          string `json:"id"`
		DefaultEmail string `json:"default_email"`
	}
	if err := json.NewDecoder(infoResp.Body).Decode(&info); err != nil {
		return "", "", fmt.Errorf("yandex info: %w", err)
	}
	if info.ID == "" || info.DefaultEmail == "" {
		return "", "", fmt.Errorf("yandex info: нет id или email")
	}
	return info.ID, info.DefaultEmail, nil
}

// takeState проверяет и съедает state (одноразовость + TTL).
func (y *Yandex) takeState(state string) bool {
	y.state.Lock()
	defer y.state.Unlock()
	exp, ok := y.state.m[state]
	if !ok || time.Now().After(exp) {
		delete(y.state.m, state)
		return false
	}
	delete(y.state.m, state)
	// подчистка протухших
	for s, e := range y.state.m {
		if time.Now().After(e) {
			delete(y.state.m, s)
		}
	}
	return true
}
