package service

import (
	"strings"
	"testing"
)

func TestParseAtTime(t *testing.T) {
	ok := []struct {
		in   string
		h, m int
	}{
		{"00:00", 0, 0}, {"2:30", 2, 30}, {"23:59", 23, 59}, {"09:05", 9, 5},
	}
	for _, c := range ok {
		h, m, err := parseAtTime(c.in)
		if err != nil || h != c.h || m != c.m {
			t.Errorf("parseAtTime(%q) = %d,%d,%v; want %d,%d,nil", c.in, h, m, err, c.h, c.m)
		}
	}
	bad := []string{"", "2", "24:00", "12:60", "-1:00", "aa:bb", "1:2:3"}
	for _, in := range bad {
		if _, _, err := parseAtTime(in); err == nil {
			t.Errorf("parseAtTime(%q) should have errored", in)
		}
	}
}

func TestPickRandomTime(t *testing.T) {
	for i := 0; i < 200; i++ {
		h, m := pickRandomTime()
		if h < 0 || h > 2 || m < 0 || m > 59 {
			t.Fatalf("pickRandomTime() = %d:%d, out of [00:00,03:00)", h, m)
		}
	}
}

func TestGeneratePlistCalendarInterval(t *testing.T) {
	p := generatePlist("/usr/local/bin/granary", 2, 30)
	if strings.Contains(p, "StartInterval") {
		t.Error("plist should no longer use StartInterval")
	}
	if !strings.Contains(p, "StartCalendarInterval") {
		t.Error("plist should use StartCalendarInterval")
	}
	for _, want := range []string{
		"<key>Hour</key>", "<integer>2</integer>",
		"<key>Minute</key>", "<integer>30</integer>",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("plist missing %q:\n%s", want, p)
		}
	}
}
