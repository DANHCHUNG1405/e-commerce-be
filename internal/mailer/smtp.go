package mailer

import (
	"context"
	"fmt"
	"net/smtp"
	"strconv"
)

type Sender interface {
	SendPasswordReset(ctx context.Context, to, resetURL string) error
}

type SMTP struct {
	host     string
	port     string
	username string
	password string
	from     string
}

func NewSMTP(host string, port int, username, password, from string) *SMTP {
	return &SMTP{host: host, port: strconv.Itoa(port), username: username, password: password, from: from}
}

func (s *SMTP) SendPasswordReset(ctx context.Context, to, resetURL string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.host == "" || s.from == "" {
		return fmt.Errorf("smtp is not configured")
	}
	auth := smtp.Auth(nil)
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}
	body := "To: " + to + "\r\n" +
		"Subject: Password reset\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		"Use this link to reset your password:\r\n" + resetURL + "\r\n"
	return smtp.SendMail(s.host+":"+s.port, auth, s.from, []string{to}, []byte(body))
}
