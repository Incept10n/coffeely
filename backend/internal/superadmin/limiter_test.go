package superadmin

import (
	"testing"
	"time"
)

func TestLimiterBlocksAndRecovers(t *testing.T) {
	l := NewLimiter(2, time.Minute)
	now := time.Unix(1_800_000_000, 0)
	l.now = func() time.Time { return now }

	if !l.Allowed("1.2.3.4") {
		t.Fatal("чистый ключ должен быть доступен")
	}
	l.Fail("1.2.3.4")
	l.Fail("1.2.3.4")
	if l.Allowed("1.2.3.4") {
		t.Fatal("после лимита попытки должны блокироваться")
	}
	if !l.Allowed("5.6.7.8") {
		t.Fatal("другой ключ не должен страдать")
	}

	now = now.Add(2 * time.Minute)
	if !l.Allowed("1.2.3.4") {
		t.Fatal("после окна ключ должен разблокироваться")
	}
}

func TestLimiterReset(t *testing.T) {
	l := NewLimiter(1, time.Minute)
	l.Fail("ip")
	if l.Allowed("ip") {
		t.Fatal("ждём блокировку")
	}
	l.Reset("ip")
	if !l.Allowed("ip") {
		t.Fatal("reset должен снимать блокировку")
	}
}

func TestParseHM(t *testing.T) {
	cases := map[string]int{
		"08:00": 8 * 60,
		"8:05":  8*60 + 5,
		"23:59": 23*60 + 59,
	}
	for in, want := range cases {
		got, err := parseHM(in)
		if err != nil || got != want {
			t.Fatalf("parseHM(%q) = %d, %v; ждём %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "8", "aa:bb", "25:00", "10:61"} {
		if _, err := parseHM(bad); err == nil {
			t.Fatalf("parseHM(%q): ждём ошибку", bad)
		}
	}
}
