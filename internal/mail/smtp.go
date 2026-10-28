package mail

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
)

// ErrInvalidMessage means the message can never be sent (e.g. a header value
// contains a line break); retrying will not help.
var ErrInvalidMessage = errors.New("invalid message")

// Sender sends one plain-text email.
type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// IsPermanent reports whether retrying err is pointless: a 5xx SMTP reply
// (e.g. 550 no such user) or an invalid message. Everything else (4xx replies,
// timeouts, connection errors) is treated as transient.
func IsPermanent(err error) bool {
	if errors.Is(err, ErrInvalidMessage) {
		return true
	}
	var tpErr *textproto.Error
	return errors.As(err, &tpErr) && tpErr.Code >= 500
}

// SMTPSender talks plain SMTP without authentication or TLS; it is meant for
// a local relay or catcher such as Mailpit.
type SMTPSender struct {
	addr string
	from string
}

func NewSMTPSender(addr, from string) *SMTPSender {
	return &SMTPSender{addr: addr, from: from}
}

func (s *SMTPSender) Send(ctx context.Context, to, subject, body string) error {
	for _, v := range []string{to, subject, s.from} {
		if strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("%w: line break in header value", ErrInvalidMessage)
		}
	}

	deadline := time.Now().Add(15 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}

	dialer := net.Dialer{Deadline: deadline}
	conn, err := dialer.DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return fmt.Errorf("connect to SMTP server: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(deadline)

	host, _, _ := net.SplitHostPort(s.addr)
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("start SMTP session: %w", err)
	}
	defer client.Close()

	if err := client.Mail(s.from); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("RCPT TO: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}

	msg := "From: " + s.from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" + body + "\r\n"
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("finish message: %w", err)
	}

	return client.Quit()
}
