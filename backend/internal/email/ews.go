// Package email — отправка писем через Exchange Web Services (EWS) с NTLM.
//
// Рабочий контур inceptech: Exchange на mail.inceptech.ru, ящик coffeely@inceptech.ru,
// NTLM-аутентификация. exchangelib из примера заменён на прямой SOAP-вызов EWS
// CreateItem(SendAndSaveCopy): так же ходит Python-библиотека, только без лишних
// зависимостей — один NTLM-roundtripper.
package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Azure/go-ntlmssp"

	"github.com/Incept10n/coffeely/backend/internal/config"
)

// EWSSender отправляет письма от ящика, под которым авторизуется в Exchange.
type EWSSender struct {
	cfg    config.Config
	client *http.Client
}

// NewEWS собирает клиент с NTLM и (по умолчанию) отключённой проверкой
// сертификата — почтовый сервер inceptech использует внутренний сертификат,
// как и в проверенном Python-примере. Включается обратно
// MAIL_INSECURE_SKIP_VERIFY=false.
func NewEWS(cfg config.Config) *EWSSender {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.MailInsecureSkipVerify}, //nolint:gosec // внутренний Exchange
	}
	return &EWSSender{
		cfg: cfg,
		client: &http.Client{
			// go-ntlmssp v0.1.x берёт учётные данные из req.SetBasicAuth
			// и сам превращает их в NTLM-рукопожатие.
			Transport: &ntlmssp.Negotiator{RoundTripper: transport},
			Timeout:   cfg.MailTimeout,
		},
	}
}

// SendVerificationCode отправляет код подтверждения на to.
func (s *EWSSender) SendVerificationCode(ctx context.Context, to, code string) error {
	subject := "Код подтверждения coffeely"
	html := fmt.Sprintf(`<!doctype html>
<html lang="ru"><body style="margin:0;padding:24px;background:#f6f1e7;font-family:-apple-system,Segoe UI,Roboto,Arial,sans-serif;color:#2b1d14">
  <div style="max-width:480px;margin:0 auto;background:#fffaf2;border-radius:16px;padding:28px">
    <p style="margin:0 0 8px;font-size:22px;font-weight:700">coffeely</p>
    <p style="margin:0 0 20px;font-size:14px;color:#6b5a4c">кофе к точному времени готовности</p>
    <p style="margin:0 0 12px;font-size:15px">Код подтверждения email:</p>
    <p style="margin:0 0 20px;font-size:34px;font-weight:700;letter-spacing:8px">%s</p>
    <p style="margin:0;font-size:13px;color:#6b5a4c">Если вы не регистрировались в coffeely, просто проигнорируйте письмо.</p>
  </div>
</body></html>`, code)

	body := createItemEnvelope(s.cfg, to, subject, html)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.MailEWSURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("email: запрос EWS: %w", err)
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("Accept", "text/xml")
	req.SetBasicAuth(s.cfg.MailUsername, s.cfg.MailPassword)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("email: EWS: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("email: EWS ответил %d: %s", resp.StatusCode, snippet(raw))
	}
	// EWS отдаёт 200 и на логические ошибки: код ответа внутри SOAP.
	if strings.Contains(string(raw), "soap:Fault") || !strings.Contains(string(raw), "ResponseCode>NoError<") {
		return fmt.Errorf("email: EWS не отправил письмо: %s", snippet(raw))
	}
	return nil
}

// createItemEnvelope собирает SOAP-запрос CreateItem для одного письма.
// CreateItem с MessageDisposition=SendAndSaveCopy — штатный способ отправки в EWS.
func createItemEnvelope(cfg config.Config, to, subject, html string) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	b.WriteString(`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"`)
	b.WriteString(` xmlns:t="http://schemas.microsoft.com/exchange/services/2006/types"`)
	b.WriteString(` xmlns:m="http://schemas.microsoft.com/exchange/services/2006/messages">`)
	b.WriteString(`<soap:Header><t:RequestServerVersion Version="Exchange2013"/></soap:Header><soap:Body>`)
	b.WriteString(`<m:CreateItem MessageDisposition="SendAndSaveCopy">`)
	b.WriteString(`<m:SavedItemFolderId><t:DistinguishedFolderId Id="sentitems"/></m:SavedItemFolderId>`)
	b.WriteString(`<m:Items><t:Message>`)
	b.WriteString(`<t:Subject>` + xmlEscape(subject) + `</t:Subject>`)
	b.WriteString(`<t:Body BodyType="HTML">` + xmlEscape(html) + `</t:Body>`)
	b.WriteString(`<t:ToRecipients><t:Mailbox><t:EmailAddress>` + xmlEscape(to) + `</t:EmailAddress></t:Mailbox></t:ToRecipients>`)
	b.WriteString(`<t:From><t:Mailbox>`)
	if cfg.MailFromName != "" {
		b.WriteString(`<t:Name>` + xmlEscape(cfg.MailFromName) + `</t:Name>`)
	}
	b.WriteString(`<t:EmailAddress>` + xmlEscape(cfg.MailFrom) + `</t:EmailAddress></t:Mailbox></t:From>`)
	b.WriteString(`</t:Message></m:Items></m:CreateItem></soap:Body></soap:Envelope>`)
	return []byte(b.String())
}

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func snippet(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > 500 {
		s = s[:500] + "…"
	}
	return s
}
