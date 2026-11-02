// Package mailer sends transactional emails.
package mailer

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/jatinjkagrawal/user-management-poc/internal/config"
)

type Message struct {
	To       string
	Subject  string
	TextBody string
	HTMLBody string
}

type Mailer interface {
	Send(ctx context.Context, msg Message) error
}

func New(cfg config.MailConfig, log *slog.Logger) Mailer {
	if cfg.Driver == "log" {
		return &logMailer{log: log}
	}
	return &smtpMailer{cfg: cfg}
}

// ---------------------------------------------------------------------------

type smtpMailer struct{ cfg config.MailConfig }

func (m *smtpMailer) Send(ctx context.Context, msg Message) error {
	addr := net.JoinHostPort(m.cfg.Host, strconv.Itoa(m.cfg.Port))

	var auth smtp.Auth
	if m.cfg.Username != "" {
		auth = smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
	}

	done := make(chan error, 1)
	go func() { done <- smtp.SendMail(addr, auth, m.cfg.From, []string{msg.To}, buildMIME(m.cfg.From, msg)) }()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("smtp send: %w", err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func buildMIME(from string, msg Message) []byte {
	const boundary = "==BOUNDARY_UM_POC=="
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + msg.To + "\r\n")
	b.WriteString("Subject: " + msg.Subject + "\r\n")
	b.WriteString("Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n")
	b.WriteString("--" + boundary + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n")
	b.WriteString(msg.TextBody + "\r\n")
	if msg.HTMLBody != "" {
		b.WriteString("--" + boundary + "\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n")
		b.WriteString(msg.HTMLBody + "\r\n")
	}
	b.WriteString("--" + boundary + "--\r\n")
	return []byte(b.String())
}

// ---------------------------------------------------------------------------

// logMailer writes emails to the log instead of sending them (useful for tests
// and local development without an SMTP server).
type logMailer struct{ log *slog.Logger }

func (m *logMailer) Send(_ context.Context, msg Message) error {
	m.log.Info("email (log driver)", "to", msg.To, "subject", msg.Subject, "body", msg.TextBody)
	return nil
}
