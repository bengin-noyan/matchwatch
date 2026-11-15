package mail

import (
	"strings"
	"testing"
	"time"
)

func TestMessageHeaders(t *testing.T) {
	m := string(Message("no-reply@mw.com.tr", "a@b.c\r\nBcc: x@evil.com", "Şifre sıfırlama", "satır 1\nsatır 2", time.Unix(0, 0).UTC()))
	head, body, _ := strings.Cut(m, "\r\n\r\n")
	if strings.Contains(head, "\r\nBcc:") || !strings.Contains(head, "Subject: =?utf-8?q?") || body != "satır 1\r\nsatır 2" {
		t.Fatalf("message:\n%q", m)
	}
}

func TestNew(t *testing.T) {
	for raw, ok := range map[string]bool{"": true, "log": true, "smtp://u:p@smtp.example.com.tr:587": true,
		"smtp://smtp.example.com.tr": false, "http://x:1": false} {
		if _, err := New(raw, "no-reply@mw.com.tr"); (err == nil) != ok {
			t.Errorf("%q: %v", raw, err)
		}
	}
	if m, err := New("smtp://u:p@smtp.example.com.tr:587", "matchwatch <no-reply@mw.com.tr>"); err != nil || m.(smtpMailer).envelope != "no-reply@mw.com.tr" {
		t.Errorf("named sender: %v %+v", err, m)
	}
	if _, err := New("smtp://u:p@smtp.example.com.tr:587", "yok"); err == nil {
		t.Error("MAIL_FROM without an address accepted")
	}
	if m, _ := New("", "x"); m != nil {
		t.Error("empty MAIL_URL must disable mail")
	}
}
