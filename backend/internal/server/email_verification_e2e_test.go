package server

// E2E-тест подтверждения email при регистрации: письмо с кодом, вход закрыт
// до подтверждения, неверный код не пускает, верный — выдаёт токены.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type fakeMailer struct {
	mu    sync.Mutex
	codes map[string]string
	fail  bool
	sent  int
}

func (f *fakeMailer) SendVerificationCode(_ context.Context, to, code string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return context.DeadlineExceeded
	}
	if f.codes == nil {
		f.codes = make(map[string]string)
	}
	f.codes[to] = code
	f.sent++
	return nil
}

func (f *fakeMailer) code(to string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.codes[to]
}

func TestEmailVerificationE2E(t *testing.T) {
	e := newTestEnv(t)
	e.cfg.VerificationCodeTTL = 15 * time.Minute
	e.cfg.VerificationResendPeriod = time.Minute
	e.cfg.VerificationMaxAttempts = 3
	mail := &fakeMailer{}
	e.handler = NewWithMailer(e.pool, e.cfg, mail)
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)

	email := uniqueEmail()
	status, resp := e.doJSON(t, "POST", "/auth/register", "", map[string]string{"email": email, "password": "supersecret1"})
	if status != http.StatusCreated {
		t.Fatalf("register: %d: %v", status, resp)
	}
	if resp["verification_required"] != true || resp["email_sent"] != true {
		t.Fatalf("register: ждём verification_required и email_sent: %v", resp)
	}
	if _, ok := resp["tokens"]; ok {
		t.Fatalf("register: токены не должны выдаваться до подтверждения: %v", resp)
	}
	code := mail.code(email)
	if len(code) != 6 {
		t.Fatalf("код в письме: %q", code)
	}

	// Вход закрыт до подтверждения, даже с верным паролем.
	if status, body := e.doJSON(t, "POST", "/auth/login", "", map[string]string{"email": email, "password": "supersecret1"}); status != http.StatusForbidden || body["code"] != "email_not_verified" {
		t.Fatalf("login до подтверждения: %d: %v", status, body)
	}

	// Неверный код.
	if status, body := e.doJSON(t, "POST", "/auth/verify-email", "", map[string]string{"email": email, "code": "000000"}); status != http.StatusBadRequest || body["code"] != "invalid_code" {
		t.Fatalf("verify неверным кодом: %d: %v", status, body)
	}

	// Верный код → токены.
	status, resp = e.doJSON(t, "POST", "/auth/verify-email", "", map[string]string{"email": email, "code": code})
	if status != http.StatusOK {
		t.Fatalf("verify: %d: %v", status, resp)
	}
	access, _ := tokensFrom(t, resp)
	if status, _ := e.doJSON(t, "GET", "/auth/me", access, nil); status != http.StatusOK {
		t.Fatalf("me после подтверждения: %d", status)
	}

	// Теперь вход открыт.
	if status, _ := e.doJSON(t, "POST", "/auth/login", "", map[string]string{"email": email, "password": "supersecret1"}); status != http.StatusOK {
		t.Fatalf("login после подтверждения: %d", status)
	}

	// Повторная отправка сразу после регистрации — 429.
	email2 := uniqueEmail()
	if status, _ := e.doJSON(t, "POST", "/auth/register", "", map[string]string{"email": email2, "password": "supersecret1"}); status != http.StatusCreated {
		t.Fatalf("register второй: %d", status)
	}
	if status, _ := e.doJSON(t, "POST", "/auth/resend-verification", "", map[string]string{"email": email2}); status != http.StatusTooManyRequests {
		t.Fatalf("resend сразу: ждём 429, получили %d", status)
	}
}

func TestEmailVerificationMailFailureE2E(t *testing.T) {
	e := newTestEnv(t)
	e.cfg.VerificationCodeTTL = 15 * time.Minute
	e.cfg.VerificationResendPeriod = time.Minute
	e.cfg.VerificationMaxAttempts = 3
	e.handler = NewWithMailer(e.pool, e.cfg, &fakeMailer{fail: true})
	e.srv = httptest.NewServer(e.handler)
	t.Cleanup(e.srv.Close)

	status, resp := e.doJSON(t, "POST", "/auth/register", "", map[string]string{"email": uniqueEmail(), "password": "supersecret1"})
	if status != http.StatusCreated || resp["email_sent"] != false {
		t.Fatalf("register при сбое почты: %d: %v", status, resp)
	}
}
