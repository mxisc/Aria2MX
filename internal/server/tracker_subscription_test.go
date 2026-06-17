package server

import (
	"strings"
	"testing"
)

func TestParseTrackerSubscriptionList(t *testing.T) {
	input := strings.NewReader(`
# comment
udp://tracker.example.com:6969/announce
http://tracker.example.net:80/announce
udp://tracker.example.com:6969/announce
ftp://invalid.example.com/announce
not-a-url
https://tracker.example.org:443/announce
`)

	got, err := parseTrackerSubscriptionList(input)
	if err != nil {
		t.Fatalf("parse tracker list: %v", err)
	}
	want := []string{
		"udp://tracker.example.com:6969/announce",
		"http://tracker.example.net:80/announce",
		"https://tracker.example.org:443/announce",
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d trackers, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected tracker %d to be %q, got %q", i, want[i], got[i])
		}
	}
}
