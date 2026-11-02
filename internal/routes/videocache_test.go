package routes

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"git.nadeko.net/Fijxu/ytproxy-go/internal/config"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/httpc"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/log"
)

const videoCacheTestSigned = "&sparams=expire,itag,clen,lmt&expire=9999999999&lmt=123&mime=video/mp4"

func videoCacheTestRequest(t *testing.T, itag, extra string, size, from, to int) *httptest.ResponseRecorder {
	t.Helper()
	target := "/videoplayback?host=rr1---sn-test.googlevideo.com&itag=" + itag + "&clen=" + strconv.Itoa(size) + extra
	req := httptest.NewRequest("GET", target, nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", from, to))
	rec := httptest.NewRecorder()
	Videoplayback(rec, req)
	return rec
}

func videoCacheTestSetup(t *testing.T, size int) ([]byte, *[]string, string) {
	t.Helper()
	config.LoadConfig()
	log.StartLogger()
	dir := t.TempDir()
	config.Cfg.Video_cache_dir = dir
	data := make([]byte, size)
	rand.Read(data)
	ranges := &[]string{}
	httpc.Client = &http.Client{Transport: rangeTransport{data, ranges, &sync.Mutex{}}}
	return data, ranges, dir
}

func TestPopularFormatIsServedFromDisk(t *testing.T) {
	data, ranges, _ := videoCacheTestSetup(t, 5*chunkSize)
	from, to := 0, 4096

	videoCacheTestRequest(t, "137", videoCacheTestSigned, len(data), from, to)
	videoCacheTestRequest(t, "137", videoCacheTestSigned, len(data), from, to)
	if len(*ranges) != 2 {
		t.Fatalf("the first two viewers must reach googlevideo, got %v", *ranges)
	}

	rec := videoCacheTestRequest(t, "137", videoCacheTestSigned, len(data), from, to)
	if len(*ranges) != 2 {
		t.Fatalf("the third viewer must be served from disk, got %v", *ranges)
	}
	if rec.Code != http.StatusPartialContent || !bytes.Equal(rec.Body.Bytes(), data[from:to+1]) {
		t.Fatalf("got %d with %d bytes", rec.Code, rec.Body.Len())
	}
	if got := rec.Header().Get("Content-Range"); got != fmt.Sprintf("bytes %d-%d/%d", from, to, len(data)) {
		t.Fatalf("Content-Range: %q", got)
	}
	if rec.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("Content-Type: %q", rec.Header().Get("Content-Type"))
	}
}

func TestLongRangeIsStoredWhole(t *testing.T) {
	data, ranges, _ := videoCacheTestSetup(t, 8*chunkSize)
	first, last := 0, 3*chunkSize+50

	videoCacheTestRequest(t, "137", videoCacheTestSigned, len(data), first, 4096)
	videoCacheTestRequest(t, "137", videoCacheTestSigned, len(data), first, 4096)
	videoCacheTestRequest(t, "137", videoCacheTestSigned, len(data), first, last)
	fetched := len(*ranges)

	rec := videoCacheTestRequest(t, "137", videoCacheTestSigned, len(data), first, last)
	if len(*ranges) != fetched {
		t.Fatalf("the long range must come from disk, got %v", *ranges)
	}
	if !bytes.Equal(rec.Body.Bytes(), data[first:last+1]) {
		t.Fatalf("stored bytes differ (%d bytes)", rec.Body.Len())
	}
}

func TestOneViewerIsNotCached(t *testing.T) {
	data, ranges, _ := videoCacheTestSetup(t, 5*chunkSize)
	videoCacheTestRequest(t, "137", videoCacheTestSigned, len(data), 0, 4096)
	videoCacheTestRequest(t, "137", videoCacheTestSigned, len(data), 5000, 9000)
	videoCacheTestRequest(t, "137", videoCacheTestSigned, len(data), 5000, 9000)
	if len(*ranges) != 3 {
		t.Fatalf("one viewer's segments must not be stored, got %v", *ranges)
	}
}

func TestOnlyCachedFormatsAndSignedParamsAreStored(t *testing.T) {
	cases := []struct{ name, itag, extra string }{
		{"low resolution", "136", videoCacheTestSigned},
		{"other audio", "251", videoCacheTestSigned},
		{"unknown itag", "9999", videoCacheTestSigned},
		{"lmt not signed", "137", "&sparams=expire,itag,clen&expire=9999999999&lmt=123&mime=video/mp4"},
		{"itag not signed", "137", "&sparams=expire,clen,lmt&expire=9999999999&lmt=123&mime=video/mp4"},
		{"odd mime", "137", "&sparams=expire,itag,clen,lmt&expire=9999999999&lmt=123&mime=text/html"},
		{"download", "137", videoCacheTestSigned + "&title=x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, ranges, dir := videoCacheTestSetup(t, 5*chunkSize)
			for range 3 {
				videoCacheTestRequest(t, c.itag, c.extra, len(data), 0, 4096)
			}
			if len(*ranges) != 3 {
				t.Fatalf("must not be cached, got %v", *ranges)
			}
			if entries, _ := os.ReadDir(dir); len(entries) != 0 {
				t.Fatalf("must leave nothing on disk, got %v", entries)
			}
		})
	}
}

func TestVideoCacheFormat(t *testing.T) {
	for itag, want := range map[string]bool{
		"137": true, "248": true, "303": true, "399": true, "401": true, "313": true, "140": true,
		"136": false, "247": false, "398": false, "251": false, "18": false, "": false,
	} {
		if videoCacheFormat(itag) != want {
			t.Errorf("itag %q: want %v", itag, want)
		}
	}
}

func TestSweepDropsLeastRecentlyUsedAndOldSightings(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	write := func(name string, size int, age time.Duration) string {
		path := filepath.Join(dir, name)
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if size > 0 {
			if err := os.WriteFile(filepath.Join(path, "0-1"), make([]byte, size), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		at := now.Add(-age)
		os.Chtimes(path, at, at)
		return path
	}
	oldest := write("old", 600, 3*time.Hour)
	newest := write("new", 600, time.Hour)
	staleSighting := write("stale", 0, 2*videoCacheSeenFor)
	freshSighting := write("fresh", 0, time.Minute)

	sweepVideoCache(dir, 1000, now)

	for path, want := range map[string]bool{oldest: false, newest: true, staleSighting: false, freshSighting: true} {
		if _, err := os.Stat(path); (err == nil) != want {
			t.Errorf("%s: want present=%v", path, want)
		}
	}
}
