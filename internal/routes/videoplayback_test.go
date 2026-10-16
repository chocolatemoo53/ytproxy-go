package routes

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"git.nadeko.net/Fijxu/ytproxy-go/internal/config"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/httpc"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/log"
)

type forbiddenTransport struct{ calls *int }

func (f forbiddenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	*f.calls++
	return &http.Response{
		StatusCode: http.StatusForbidden,
		Body:       http.NoBody,
		Header:     http.Header{},
		Request:    req,
	}, nil
}

func TestRefusedURLIsHandedOverToTheCompanion(t *testing.T) {
	t.Setenv("YTPROXY_COMPANION_FALLBACK_PATH", "/companion")
	config.LoadConfig()
	log.StartLogger()
	calls := 0
	httpc.Client = &http.Client{Transport: forbiddenTransport{&calls}}

	target := "/videoplayback?host=rr1---sn-test.googlevideo.com&itag=140&expire=9999999999&ytid=abcdefghijk.2"
	rec := httptest.NewRecorder()
	Videoplayback(rec, httptest.NewRequest("GET", target, nil))

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("a refused URL must be redirected to the companion, got %d", rec.Code)
	}
	location, _ := url.Parse(rec.Header().Get("Location"))
	if !strings.HasPrefix(location.Path, "/companion/videoplayback") {
		t.Fatalf("redirected to %q", location)
	}
	q := location.Query()
	if q.Get("ytid") != "abcdefghijk" || q.Get("host") != "rr1---sn-test.googlevideo.com" || q.Get("itag") != "140" {
		t.Fatalf("the companion needs the bare video id and the original params, got %v", q)
	}

	// The same session again goes to the companion without asking googlevideo
	before := calls
	rec = httptest.NewRecorder()
	Videoplayback(rec, httptest.NewRequest("GET", target, nil))
	if rec.Code != http.StatusTemporaryRedirect || calls != before {
		t.Fatalf("a refused session must be handed over straight away, got %d after %d more requests", rec.Code, calls-before)
	}

	t.Setenv("YTPROXY_COMPANION_FALLBACK_PATH", "")
	config.LoadConfig()
	rec = httptest.NewRecorder()
	Videoplayback(rec, httptest.NewRequest("GET", target, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("without a fallback the 403 is passed on, got %d", rec.Code)
	}
}

// Serves the `range` query parameter of a fixed file, like googlevideo does.
type rangeTransport struct {
	data   []byte
	ranges *[]string
	mu     *sync.Mutex
}

func (f rangeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.URL.Query().Get("range")
	f.mu.Lock()
	*f.ranges = append(*f.ranges, r)
	f.mu.Unlock()
	var from, to int
	fmt.Sscanf(r, "%d-%d", &from, &to)
	body := f.data[from : to+1]
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Length": {strconv.Itoa(len(body))}, "Content-Type": {"video/webm"}},
		Request:    req,
	}, nil
}

func TestLongRangesAreFetchedInChunks(t *testing.T) {
	config.LoadConfig()
	log.StartLogger()
	data := make([]byte, 5*chunkSize+123)
	rand.Read(data)
	var ranges []string
	httpc.Client = &http.Client{Transport: rangeTransport{data, &ranges, &sync.Mutex{}}}

	from, to := 100, len(data)-7
	req := httptest.NewRequest("GET", "/videoplayback?host=rr1---sn-test.googlevideo.com&itag=313&expire=9999999999&clen="+strconv.Itoa(len(data)), nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", from, to))
	rec := httptest.NewRecorder()
	Videoplayback(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("want 206, got %d", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), data[from:to+1]) {
		t.Fatalf("the chunks must be stitched back in order (%d bytes)", rec.Body.Len())
	}
	if rec.Header().Get("Content-Length") != strconv.Itoa(to-from+1) {
		t.Fatalf("Content-Length must be the whole range, got %s", rec.Header().Get("Content-Length"))
	}
	if len(ranges) != 6 {
		t.Fatalf("want 6 chunk requests, got %v", ranges)
	}
	for _, r := range ranges {
		var a, b int
		fmt.Sscanf(r, "%d-%d", &a, &b)
		if b-a+1 > chunkSize {
			t.Fatalf("chunk %s is longer than %d", r, chunkSize)
		}
	}
}

func TestSetUserAgent(t *testing.T) {
	cases := []struct{ client, cver, ua string }{
		{"VISIONOS", "", constants.VisionosUserAgent},
		{"TVHTML5", "7.20260311.12.00", constants.TvUserAgent},
		{"TVHTML5", "5.20260707", constants.TvSamsungUserAgent},
		{"WEB", "2.20260708", constants.DefaultUa},
	}
	for _, c := range cases {
		h := defaultHeaders.Clone()
		setUserAgent(&h, c.client, c.cver)
		if h.Get("User-Agent") != c.ua {
			t.Errorf("%s %s: got %q", c.client, c.cver, h.Get("User-Agent"))
		}
		if native := c.client == "VISIONOS"; native == (h.Get("Origin") != "") {
			t.Errorf("%s: Origin %q", c.client, h.Get("Origin"))
		}
	}
	if defaultHeaders.Get("Origin") == "" {
		t.Error("setUserAgent changed the shared default headers")
	}
}
