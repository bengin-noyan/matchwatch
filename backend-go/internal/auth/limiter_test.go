package auth

import (
	"testing"
	"time"
)

func TestLimiterWindow(t *testing.T) {
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	l := NewLimiter(3, 15*time.Minute)
	l.now = func() time.Time { return now }
	for i := 0; i < 3; i++ {
		if l.Blocked("a@x.tr") {
			t.Fatalf("blocked after %d fails", i)
		}
		l.Fail("a@x.tr")
	}
	if !l.Blocked("a@x.tr") || l.Blocked("b@x.tr") {
		t.Fatal("limit not per key")
	}
	now = now.Add(15*time.Minute + time.Second)
	if l.Blocked("a@x.tr") {
		t.Fatal("window did not expire")
	}
	l.Fail("a@x.tr")
	l.Reset("a@x.tr")
	if l.Blocked("a@x.tr") || len(l.fails) != 0 {
		t.Fatal("reset kept state")
	}
}
