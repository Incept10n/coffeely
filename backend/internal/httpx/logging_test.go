package httpx

import "testing"

// Одноразовые OAuth-коды и токены не должны попадать в логи открытым текстом.
func TestRedactQuery(t *testing.T) {
	cases := map[string]string{
		"":                     "",
		"format=json":          "format=json",
		"code=abc&state=xyz":   "code=***&state=***",
		"format=json&code=abc": "format=json&code=***",
		"access_token=secret":  "access_token=***",
		"email=a@b.c&token=t":  "email=a@b.c&token=***",
		"codeword=keep":        "codeword=keep", // совпадение по имени, не по подстроке
	}
	for in, want := range cases {
		if got := redactQuery(in); got != want {
			t.Fatalf("redactQuery(%q) = %q, ждём %q", in, got, want)
		}
	}
}
