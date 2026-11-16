package email

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/Incept10n/coffeely/backend/internal/config"
)

func TestCreateItemEnvelope(t *testing.T) {
	cfg := config.Config{
		MailFrom:     "coffeely@inceptech.ru",
		MailFromName: "coffeely",
	}
	body := string(createItemEnvelope(cfg, "user@example.com", "Тема & <тест>", "<b>код 123456</b>"))

	var envelope struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("невалидный XML: %v\n%s", err, body)
	}
	for _, want := range []string{
		`MessageDisposition="SendAndSaveCopy"`,
		`<t:EmailAddress>user@example.com</t:EmailAddress>`,
		`<t:EmailAddress>coffeely@inceptech.ru</t:EmailAddress>`,
		`<t:Name>coffeely</t:Name>`,
		`BodyType="HTML"`,
		"Тема &amp; &lt;тест&gt;",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("в письме нет %q:\n%s", want, body)
		}
	}
	// ToRecipients должен быть внутри t:Message, а не рядом с ним.
	msgStart := strings.Index(body, "<t:Message>")
	msgEnd := strings.Index(body, "</t:Message>")
	to := strings.Index(body, "<t:ToRecipients>")
	if msgStart < 0 || msgEnd < 0 || to < msgStart || to > msgEnd {
		t.Fatalf("ToRecipients вне t:Message:\n%s", body)
	}
}
