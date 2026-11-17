// Package mail sends the invite and password reset e-mails. The provider is anything that speaks SMTP; KVKK wants it
// in Türkiye (runbook). Only the recipient's e-mail address and a one-time link leave our infrastructure.
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

// New reads MAIL_URL: "" = no mail (nil), "log" = write mails to the server log (development only, the links are
// secrets), smtp://user:pass@host:587 = an SMTP server with STARTTLS.
// shortcut: no implicit-TLS port 465; add tls.Dial when a provider offers only that.
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
	sender, err := mail.ParseAddress(from) // "matchwatch <no-reply@x.com.tr>" or a bare address
	if err != nil || strings.ContainsAny(from, "\r\n") {
		return nil, errors.New("MAIL_FROM must be an e-mail address, optionally with a name")
	}
	s := smtpMailer{addr: u.Host, from: from, envelope: sender.Address}
	if u.User != nil {
		pw, _ := u.User.Password()
		// PlainAuth refuses to send the password unless the connection is TLS (or localhost)
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
	addr, from, envelope string // from is the From header, envelope the bare address for MAIL FROM
	auth                 smtp.Auth
}

func (s smtpMailer) Send(ctx context.Context, to, subject, body string) error {
	msg := Message(s.from, to, subject, body, time.Now())
	done := make(chan error, 1)
	go func() { done <- smtp.SendMail(s.addr, s.auth, s.envelope, []string{to}, msg) }() // net/smtp takes no context
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Message builds a plain-text UTF-8 mail. Callers pass validated addresses; CR and LF are still stripped from every
// header field, so nothing can inject a header.
func Message(from, to, subject, body string, at time.Time) []byte {
	clean := func(s string) string { return strings.NewReplacer("\r", "", "\n", "").Replace(s) }
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\n", clean(from), clean(to),
		mime.QEncoding.Encode("utf-8", clean(subject)), at.Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n"))
	return []byte(b.String())
}
