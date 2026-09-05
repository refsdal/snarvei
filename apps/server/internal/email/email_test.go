package email_test

import (
	"bufio"
	"context"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/refsdal/snarvei/server/internal/email"
)

// fakeSMTP speaks just enough SMTP to accept one plaintext message.
type fakeSMTP struct {
	addr string
	mu   sync.Mutex
	data string
	from string
	rcpt string
}

func startFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{addr: ln.Addr().String()}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		w := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		w("220 fake ESMTP")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			switch {
			case strings.HasPrefix(line, "EHLO"):
				w("250-fake")
				w("250 AUTH PLAIN")
			case strings.HasPrefix(line, "AUTH PLAIN"):
				w("235 ok")
			case strings.HasPrefix(line, "MAIL FROM:"):
				f.mu.Lock()
				f.from = line
				f.mu.Unlock()
				w("250 ok")
			case strings.HasPrefix(line, "RCPT TO:"):
				f.mu.Lock()
				f.rcpt = line
				f.mu.Unlock()
				w("250 ok")
			case line == "DATA":
				w("354 go")
				var b strings.Builder
				for {
					l, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if l == ".\r\n" {
						break
					}
					b.WriteString(l)
				}
				f.mu.Lock()
				f.data = b.String()
				f.mu.Unlock()
				w("250 queued")
			case line == "QUIT":
				w("221 bye")
				return
			default:
				w("250 ok")
			}
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return f
}

func TestSMTPSendsAMultipartMessage(t *testing.T) {
	srv := startFakeSMTP(t)
	host, port, _ := net.SplitHostPort(srv.addr)
	var p int
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	sender := email.NewSMTP(email.SMTPConfig{Host: host, Port: p, Username: "u", Password: "p", From: "Snarvei <no-reply@example.com>"})
	msg := email.PasswordReset("Snarvei", "http://localhost:3000/reset-password?token=abc").To("someone@example.com")
	if err := sender.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if !strings.Contains(srv.from, "no-reply@example.com") || !strings.Contains(srv.rcpt, "someone@example.com") {
		t.Fatalf("envelope: %q %q", srv.from, srv.rcpt)
	}
	for _, want := range []string{"Subject: Reset your Snarvei password", "Content-Type: multipart/alternative", "text/plain", "text/html", "reset-password?token=abc"} {
		if !strings.Contains(srv.data, want) {
			t.Errorf("message lacks %q:\n%s", want, srv.data)
		}
	}
}

func TestNoopLogsWithoutTheBody(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	sender := email.NewNoop(logger)
	msg := email.EmailChange("Snarvei", "new@example.com", "http://localhost:3000/app/settings?emailToken=secret").To("new@example.com")
	if err := sender.Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "email.not_configured") || !strings.Contains(out, "new@example.com") {
		t.Fatalf("log line: %s", out)
	}
	if strings.Contains(out, "secret") {
		t.Fatalf("log line leaks the token: %s", out)
	}
}

func TestRecordingKeepsMessages(t *testing.T) {
	rec := email.NewRecording()
	_ = rec.Send(context.Background(), email.Invitation("Snarvei", "Acme", "Ada", "http://x/app/invitations/1").To("a@example.com"))
	_ = rec.Send(context.Background(), email.Invitation("Snarvei", "Acme", "", "http://x/app/invitations/2").To("b@example.com"))
	if len(rec.Messages()) != 2 {
		t.Fatal("expected two messages")
	}
	last, ok := rec.Last("b@example.com")
	if !ok || !strings.Contains(last.Text, "/app/invitations/2") || strings.Contains(last.Text, " by ") {
		t.Fatalf("last for b: %+v", last)
	}
	first, _ := rec.Last("a@example.com")
	if !strings.Contains(first.Text, "invited by Ada") {
		t.Fatalf("inviter missing: %q", first.Text)
	}
	rec.Reset()
	if len(rec.Messages()) != 0 {
		t.Fatal("reset must clear")
	}
}

func TestTemplatesEscapeHTML(t *testing.T) {
	tpl := email.Invitation("Snarvei", "<b>Acme</b>", "", "http://x/app/invitations/1")
	if strings.Contains(tpl.HTML, "<b>Acme</b>") || !strings.Contains(tpl.HTML, "&lt;b&gt;Acme&lt;/b&gt;") {
		t.Fatalf("org name not escaped: %s", tpl.HTML)
	}
	if !strings.Contains(tpl.Subject, "<b>Acme</b>") {
		t.Fatal("subject is plain text and must not be escaped")
	}
}
