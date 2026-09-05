package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// SMTPConfig is the all-or-nothing SMTP group from the environment.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string // "Name <addr>" or "addr"
}

type smtpSender struct{ cfg SMTPConfig }

// NewSMTP returns a Sender that speaks SMTP: implicit TLS on 465, STARTTLS
// when the server offers it otherwise (required unless the host is loopback),
// PLAIN auth when a username is set.
func NewSMTP(cfg SMTPConfig) Sender { return &smtpSender{cfg: cfg} }

func (s *smtpSender) Send(ctx context.Context, m Message) error {
	from, err := mail.ParseAddress(s.cfg.From)
	if err != nil {
		return fmt.Errorf("email: EMAIL_FROM %q: %w", s.cfg.From, err)
	}
	to, err := mail.ParseAddress(m.To)
	if err != nil {
		return fmt.Errorf("email: recipient %q: %w", m.To, err)
	}
	body, err := build(from, to, m)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	if s.cfg.Port == 465 {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: s.cfg.Host})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("email: connect %s: %w", addr, err)
	}
	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("email: smtp handshake: %w", err)
	}
	defer client.Close()

	tlsOn := s.cfg.Port == 465
	if s.cfg.Port != 465 {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: s.cfg.Host}); err != nil {
				return fmt.Errorf("email: starttls: %w", err)
			}
			tlsOn = true
		} else if !isLoopback(s.cfg.Host) {
			return fmt.Errorf("email: %s offers no STARTTLS; refusing to send credentials in clear", s.cfg.Host)
		}
	}
	if s.cfg.Username != "" {
		if !tlsOn && !isLoopback(s.cfg.Host) {
			return fmt.Errorf("email: refusing to authenticate to %s without TLS", s.cfg.Host)
		}
		if err := client.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("email: auth: %w", err)
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("email: MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to.Address); err != nil {
		return fmt.Errorf("email: RCPT TO: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("email: DATA: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("email: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("email: finish body: %w", err)
	}
	return client.Quit()
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// build renders RFC 5322 headers plus a multipart/alternative body.
func build(from, to *mail.Address, m Message) ([]byte, error) {
	var buf bytes.Buffer
	mp := multipart.NewWriter(&buf)
	fmt.Fprintf(&buf, "From: %s\r\n", from.String())
	fmt.Fprintf(&buf, "To: %s\r\n", to.String())
	fmt.Fprintf(&buf, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", m.Subject))
	fmt.Fprintf(&buf, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", mp.Boundary())
	part := func(ctype, content string) error {
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", ctype+"; charset=utf-8")
		h.Set("Content-Transfer-Encoding", "8bit")
		w, err := mp.CreatePart(h)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(strings.ReplaceAll(content, "\n", "\r\n")))
		return err
	}
	if err := part("text/plain", m.Text); err != nil {
		return nil, fmt.Errorf("email: text part: %w", err)
	}
	if m.HTML != "" {
		if err := part("text/html", m.HTML); err != nil {
			return nil, fmt.Errorf("email: html part: %w", err)
		}
	}
	if err := mp.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
