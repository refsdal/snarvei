package main

import "testing"

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
