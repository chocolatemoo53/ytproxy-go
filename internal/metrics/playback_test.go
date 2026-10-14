package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestPlaybackCountsRecentSessionsAndFormats(t *testing.T) {
	p := &playback{seen: map[stream]streamSeen{}}
	start := time.Unix(1000, 0)
	p.seenAt(start, "a", "248", "video/webm")
	p.seenAt(start, "a", "251", "audio/webm")
	p.seenAt(start, "a", "248", "video/webm")
	p.seenAt(start.Add(30*time.Second), "b", "248", "video/webm")
	p.seenAt(start.Add(30*time.Second), "c", "9999", "video/mp4")
	p.seenAt(start, "", "248", "video/webm")

	sessions, streams := p.snapshot(start.Add(45 * time.Second))
	if sessions != 3 {
		t.Fatalf("sessions = %d, want 3", sessions)
	}
	want := map[[2]string]int{
		{"video", "1080p VP9"}: 2,
		{"audio", "Opus 160k"}: 1,
		{"video", "itag 9999"}: 1,
	}
	if len(streams) != len(want) {
		t.Fatalf("streams = %v, want %v", streams, want)
	}
	for labels, n := range want {
		if streams[labels] != n {
			t.Fatalf("streams = %v, want %v", streams, want)
		}
	}

	sessions, _ = p.snapshot(start.Add(80 * time.Second))
	if sessions != 2 {
		t.Fatalf("after session a went quiet: sessions = %d, want 2", sessions)
	}
}

func TestPlaybackIsRegistered(t *testing.T) {
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() == "http3_ytproxy_playback_sessions" {
			return
		}
	}
	t.Fatal("http3_ytproxy_playback_sessions is not exported")
}
