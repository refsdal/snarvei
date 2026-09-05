package redirect

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/db/gen"
)

// ClickEvent is one sanitised click.
type ClickEvent struct {
	LinkID, Slug, IPHash string
	UserAgent            *string
	Referer              *string
	QueryString          *string
	Country              *string
	Host, Path           string
	RedirectStatus       int
}

// insertTimeout bounds one click insert; a stuck database must not pin
// goroutines forever.
const insertTimeout = 5 * time.Second

// Recorder stores clicks asynchronously and can be drained at shutdown.
type Recorder struct {
	q   *gen.Queries
	log *slog.Logger
	wg  sync.WaitGroup
}

// NewRecorder builds a Recorder over q.
func NewRecorder(q *gen.Queries, log *slog.Logger) *Recorder {
	if log == nil {
		log = slog.Default()
	}
	return &Recorder{q: q, log: log}
}

// Record inserts e in the background. Analytics must never break a redirect,
// so failures are logged as click.record_failed and otherwise ignored.
func (r *Recorder) Record(e ClickEvent) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), insertTimeout)
		defer cancel()
		err := r.q.InsertClick(ctx, gen.InsertClickParams{
			ID: auth.NewID(), LinkID: e.LinkID, IpHash: e.IPHash, UserAgent: e.UserAgent, Referer: e.Referer,
			Country: e.Country, Host: e.Host, Path: e.Path, QueryString: e.QueryString, RedirectStatusUsed: int16(e.RedirectStatus),
		})
		if err != nil {
			r.log.Error("click not recorded", "event", "click.record_failed", "link", e.LinkID, "slug", e.Slug, "error", err.Error())
		}
	}()
}

// Drain waits for in-flight inserts up to timeout.
func (r *Recorder) Drain(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}
