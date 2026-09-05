// Package ratelimit is Snarvei's fixed-window counter over the rate_limit
// table, shared by every replica. Keys are built by Key so the window index
// is part of the key and buckets never outlive their window.
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/refsdal/snarvei/server/internal/db/gen"
)

// Store counts hits per key.
type Store interface {
	// Hit increments the bucket for key in the window containing now and
	// returns the count after the increment plus how long until the window
	// ends (what a 429 puts in Retry-After).
	Hit(ctx context.Context, key string, window time.Duration) (count int, retryAfter time.Duration, err error)
}

// Key builds "rl:<name>:<bucket>:<windowIndex>" and returns the window start.
func Key(name, bucket string, now time.Time, window time.Duration) (string, time.Time) {
	index := now.Unix() / int64(window/time.Second)
	start := time.Unix(index*int64(window/time.Second), 0).UTC()
	return fmt.Sprintf("rl:%s:%s:%d", name, bucket, index), start
}

// Postgres is the production Store.
type Postgres struct {
	q   *gen.Queries
	now func() time.Time
}

// NewPostgres builds a Store over q.
func NewPostgres(q *gen.Queries) Store { return &Postgres{q: q, now: time.Now} }

func (p *Postgres) Hit(ctx context.Context, key string, window time.Duration) (int, time.Duration, error) {
	now := p.now()
	_, start := Key("", "", now, window)
	count, err := p.q.HitRateLimit(ctx, gen.HitRateLimitParams{
		Key:         key,
		WindowStart: pgtype.Timestamptz{Time: start, Valid: true},
	})
	if err != nil {
		return 0, 0, fmt.Errorf("ratelimit: hit %q: %w", key, err)
	}
	// Opportunistic housekeeping on the first hit of a bucket: drop buckets
	// older than two windows. Failure is not the caller's problem.
	if count == 1 {
		cutoff := start.Add(-2 * window)
		_, _ = p.q.SweepRateLimit(ctx, pgtype.Timestamptz{Time: cutoff, Valid: true})
	}
	retry := start.Add(window).Sub(now)
	if retry <= 0 {
		retry = time.Second
	}
	return int(count), retry, nil
}
