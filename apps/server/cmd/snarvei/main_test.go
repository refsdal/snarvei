package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/refsdal/snarvei/server/internal/config"
	"github.com/refsdal/snarvei/server/internal/testrig"
)

func TestParseArgs(t *testing.T) {
	cases := []struct {
		args []string
		want dispatchMode
	}{
		{nil, modeDefault},
		{[]string{""}, modeDefault},
		{[]string{"server"}, modeServer},
		{[]string{"migrate"}, modeMigrate},
		{[]string{"migrations"}, modeMigrate},
		{[]string{"healthcheck"}, modeHealthcheck},
		{[]string{"migrationz"}, modeUnknown},
	}
	for _, c := range cases {
		if got := parseArgs(c.args); got.mode != c.want {
			t.Errorf("parseArgs(%v) = %v, want %v", c.args, got.mode, c.want)
		}
	}
}

func TestUnknownModeExits2(t *testing.T) {
	if code := run([]string{"bogus"}); code != 2 {
		t.Fatalf("run(bogus) = %d, want 2", code)
	}
}

func TestHealthcheckAgainstNothingExits1(t *testing.T) {
	if code := healthcheckMode("1"); code != 1 {
		t.Fatalf("healthcheck on a closed port = %d, want 1", code)
	}
}

// TestServeLifecycle drives the boot/shutdown path serveMode delegates to:
// listen, answer /healthz (including the positive healthcheckMode path),
// then shut down cleanly on SIGTERM with the listener closed afterward.
func TestServeLifecycle(t *testing.T) {
	testrig.Setup(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("release reserved port: %v", err)
	}

	env := map[string]string{
		"DATABASE_URL":    testrig.DatabaseURL(),
		"APP_URL":         "http://127.0.0.1:0",
		"AUTH_SECRET":     strings.Repeat("a", 32),
		"STORAGE_DRIVER":  "fs",
		"STORAGE_FS_PATH": t.TempDir(),
		"PORT":            strconv.Itoa(port),
	}
	cfg, err := config.Load(env)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	sig := make(chan os.Signal, 1)
	done := make(chan int, 1)
	go func() { done <- serve(cfg, false, sig) }()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(5 * time.Second)
	healthy := false
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://" + addr + "/healthz")
		if err == nil {
			healthy = resp.StatusCode >= 200 && resp.StatusCode < 300
			resp.Body.Close()
			if healthy {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !healthy {
		t.Fatal("server never became healthy within 5s")
	}

	if code := healthcheckMode(strconv.Itoa(port)); code != 0 {
		t.Fatalf("healthcheckMode(%d) = %d, want 0", port, code)
	}

	sig <- syscall.SIGTERM

	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("serve() returned %d after SIGTERM, want 0", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not return within 10s of SIGTERM")
	}

	if conn, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
		conn.Close()
		t.Fatal("port still accepting connections after shutdown")
	}
}
