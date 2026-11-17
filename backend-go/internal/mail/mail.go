// Package mail, davet ve şifre sıfırlama e-postalarını gönderir. SMTP konuşan her sağlayıcı olur; KVKK gereği sağlayıcı
// Türkiye'de olmalı (runbook). Altyapımızdan yalnızca alıcının e-posta adresi ve tek kullanımlık bağlantı çıkar.
package mail

import (
	"context"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/mail"
	"net/smtp"
	"net/url"
	"strings"
	"time"
)

type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// New, MAIL_URL'yi okur: "" ise e-posta yok (nil); "log" ise e-postalar sunucu loguna yazılır (yalnızca geliştirme
// için, çünkü bağlantılar gizli); smtp://user:pass@host:587 ise STARTTLS'li bir SMTP sunucusu kullanılır.
// shortcut: 465 portu (doğrudan TLS) desteklenmiyor; yalnızca onu sunan bir sağlayıcı seçersek tls.Dial ekle.
func New(raw, from string) (Mailer, error) {
	switch raw {
	case "":
		return nil, nil
	case "log":
		return logMailer{}, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "smtp" || u.Hostname() == "" || u.Port() == "" {
		return nil, errors.New("MAIL_URL must be smtp://user:password@host:port or log")
	}
	sender, err := mail.ParseAddress(from) // "matchwatch <no-reply@x.com.tr>" ya da yalnızca adres
	if err != nil || strings.ContainsAny(from, "\r\n") {
		return nil, errors.New("MAIL_FROM must be an e-mail address, optionally with a name")
	}
	s := smtpMailer{addr: u.Host, from: from, envelope: sender.Address}
	if u.User != nil {
		pw, _ := u.User.Password()
		// PlainAuth, bağlantı TLS değilse (ya da localhost değilse) şifreyi göndermez
		s.auth = smtp.PlainAuth("", u.User.Username(), pw, u.Hostname())
	}
	return s, nil
}

type logMailer struct{}

func (logMailer) Send(_ context.Context, to, subject, body string) error {
	log.Printf("mail (MAIL_URL=log) to %s: %s\n%s", to, subject, body)
	return nil
}

type smtpMailer struct {
	addr, from, envelope string // from, From header'ı; envelope ise MAIL FROM için yalın adres
	auth                 smtp.Auth
}

func (s smtpMailer) Send(ctx context.Context, to, subject, body string) error {
	msg := Message(s.from, to, subject, body, time.Now())
	done := make(chan error, 1)
	go func() { done <- smtp.SendMail(s.addr, s.auth, s.envelope, []string{to}, msg) }() // net/smtp context almıyor
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Message, düz metin UTF-8 bir e-posta oluşturur. Çağıranlar doğrulanmış adres verir; yine de her header alanından CR
// ve LF silinir ki kimse header ekleyemesin.
func Message(from, to, subject, body string, at time.Time) []byte {
	clean := func(s string) string { return strings.NewReplacer("\r", "", "\n", "").Replace(s) }
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\n", clean(from), clean(to),
		mime.QEncoding.Encode("utf-8", clean(subject)), at.Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n"))
	return []byte(b.String())
}
