// Package email is the transactional mail port: one Sender interface, an
// SMTP driver, a no-op driver that logs a redacted line when mail is not
// configured, a recording driver for tests and e2e hooks, and the templates.
package email

import (
	"context"
	"log/slog"
	"sync"
)

// Message is one outgoing mail.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// Sender delivers messages.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

type noop struct{ log *slog.Logger }

// NewNoop returns a Sender that drops every message and logs
// event=email.not_configured with only the recipient and subject: bodies
// carry bearer links and must never be logged.
func NewNoop(log *slog.Logger) Sender {
	if log == nil {
		log = slog.Default()
	}
	return &noop{log: log}
}

func (n *noop) Send(_ context.Context, m Message) error {
	n.log.Warn("email dropped: no SMTP configured", "event", "email.not_configured", "to", m.To, "subject", m.Subject)
	return nil
}

// Recording keeps every message in memory (tests, E2E_TEST_HOOKS).
type Recording struct {
	mu       sync.Mutex
	messages []Message
}

// NewRecording returns an empty Recording.
func NewRecording() *Recording { return &Recording{} }

func (r *Recording) Send(_ context.Context, m Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, m)
	return nil
}

// Messages returns a copy, oldest first.
func (r *Recording) Messages() []Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Message, len(r.messages))
	copy(out, r.messages)
	return out
}

// Last returns the most recent message sent to addr.
func (r *Recording) Last(addr string) (Message, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.messages) - 1; i >= 0; i-- {
		if r.messages[i].To == addr {
			return r.messages[i], true
		}
	}
	return Message{}, false
}

// Reset forgets everything.
func (r *Recording) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = nil
}
