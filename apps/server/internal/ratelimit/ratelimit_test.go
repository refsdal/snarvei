package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/ratelimit"
	"github.com/refsdal/snarvei/server/internal/testrig"
)

func TestKeyIsStableWithinAWindow(t *testing.T) {
	base := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	k1, ws1 := ratelimit.Key("redirect", "abc", base, time.Minute)
	k2, _ := ratelimit.Key("redirect", "abc", base.Add(30*time.Second), time.Minute)
	k3, ws3 := ratelimit.Key("redirect", "abc", base.Add(61*time.Second), time.Minute)
	if k1 != k2 || k1 == k3 {
		t.Fatalf("keys: %s %s %s", k1, k2, k3)
	}
	if !ws1.Equal(base) || !ws3.Equal(base.Add(time.Minute)) {
		t.Fatalf("window starts: %v %v", ws1, ws3)
	}
}

func TestPostgresHitCountsAndSweeps(t *testing.T) {
	rig := testrig.Setup(t)
	store := ratelimit.NewPostgres(gen.New(rig.Pool))
	ctx := context.Background()

	var last int
	for i := 1; i <= 3; i++ {
		count, retry, err := store.Hit(ctx, "rl:test:bucket:1", time.Minute)
		if err != nil {
			t.Fatalf("hit %d: %v", i, err)
		}
		if count != i {
			t.Fatalf("hit %d counted %d", i, count)
		}
		if retry <= 0 || retry > time.Minute {
			t.Fatalf("retryAfter = %v", retry)
		}
		last = count
	}
	if last != 3 {
		t.Fatal("expected 3 hits")
	}

	// An old bucket is swept when a new window opens.
	if _, err := rig.Pool.Exec(ctx, `INSERT INTO rate_limit (key, window_start, count) VALUES ('rl:old', now() - interval '1 hour', 9)`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Hit(ctx, "rl:test:other:1", time.Minute); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := rig.Pool.QueryRow(ctx, `SELECT count(*) FROM rate_limit WHERE key = 'rl:old'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("old bucket was not swept")
	}
}
