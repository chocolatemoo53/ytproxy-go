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

func TestChunkedRange(t *testing.T) {
	size := strconv.Itoa(10 * chunkSize)
	last := int64(10*chunkSize - 1)
	cases := []struct {
		requestBytes, clen string
		from, to           int64
		ok                 bool
	}{
		{"", size, 0, last, true},
		{"", "", 0, 0, false},
		{"0-", size, 0, last, true},
		{"500-", size, 500, last, true},
		{"500-", "", 0, 0, false},
		{"0-" + strconv.Itoa(chunkSize*3), "", 0, chunkSize * 3, true},
		{"0-" + strconv.Itoa(chunkSize*30), size, 0, last, true},
		{"0-99", size, 0, 99, false},
		{"-500", size, 0, 0, false},
		{strconv.Itoa(20*chunkSize) + "-", size, 0, 0, false},
	}
	for _, c := range cases {
		from, to, ok := chunkedRange(c.requestBytes, c.clen)
		if ok != c.ok || (ok && (from != c.from || to != c.to)) {
			t.Errorf("%q clen %q: got %d-%d %v", c.requestBytes, c.clen, from, to, ok)
		}
	}
}

func TestOpenAndMissingRangesAreFetchedInChunks(t *testing.T) {
	config.LoadConfig()
	log.StartLogger()
	data := make([]byte, 5*chunkSize+123)
	rand.Read(data)
	size := len(data)
	cases := []struct {
		rangeHeader  string
		from         int
		code         int
		contentRange string
	}{
		{"", 0, http.StatusOK, ""},
		{"bytes=0-", 0, http.StatusPartialContent, fmt.Sprintf("bytes 0-%d/%d", size-1, size)},
		{"bytes=100-", 100, http.StatusPartialContent, fmt.Sprintf("bytes 100-%d/%d", size-1, size)},
		{fmt.Sprintf("bytes=100-%d", size+999), 100, http.StatusPartialContent, fmt.Sprintf("bytes 100-%d/%d", size-1, size)},
	}
	for _, c := range cases {
		var ranges []string
		httpc.Client = &http.Client{Transport: rangeTransport{data, &ranges, &sync.Mutex{}}}
		req := httptest.NewRequest("GET", "/videoplayback?host=rr1---sn-test.googlevideo.com&itag=313&expire=9999999999&title=video.mp4&clen="+strconv.Itoa(size), nil)
		if c.rangeHeader != "" {
			req.Header.Set("Range", c.rangeHeader)
		}
		rec := httptest.NewRecorder()
		Videoplayback(rec, req)
		if rec.Code != c.code {
			t.Fatalf("%q: want %d, got %d", c.rangeHeader, c.code, rec.Code)
		}
		if !bytes.Equal(rec.Body.Bytes(), data[c.from:]) {
			t.Fatalf("%q: wrong body (%d bytes)", c.rangeHeader, rec.Body.Len())
		}
		if got := rec.Header().Get("Content-Range"); got != c.contentRange {
			t.Fatalf("%q: Content-Range %q", c.rangeHeader, got)
		}
		if got := rec.Header().Get("Content-Length"); got != strconv.Itoa(size-c.from) {
			t.Fatalf("%q: Content-Length %s", c.rangeHeader, got)
		}
		if !strings.Contains(rec.Header().Get("Content-Disposition"), "video.mp4") {
			t.Fatalf("%q: lost the download filename", c.rangeHeader)
		}
		if len(ranges) < 2 {
			t.Fatalf("%q: want chunk requests, got %v", c.rangeHeader, ranges)
		}
	}
}

func TestHeadIsNotChunked(t *testing.T) {
	if _, _, ok := chunkedRange("", "1000"); ok {
		t.Fatal("a file under one chunk must not be chunked")
	}
	config.LoadConfig()
	log.StartLogger()
	data := make([]byte, 5*chunkSize)
	var ranges []string
	httpc.Client = &http.Client{Transport: rangeTransport{data, &ranges, &sync.Mutex{}}}
	req := httptest.NewRequest("HEAD", "/videoplayback?host=rr1---sn-test.googlevideo.com&itag=313&expire=9999999999&clen="+strconv.Itoa(len(data)), nil)
	Videoplayback(httptest.NewRecorder(), req)
	if len(ranges) > 1 {
		t.Fatalf("HEAD must not fetch chunks, got %v", ranges)
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

func TestHostFromServerParams(t *testing.T) {
	cases := map[string]string{
		"fvip=5&mn=sn-vgqskndl%2Csn-vgqsrnd7": "r5---sn-vgqskndl.googlevideo.com",
		"fvip=12&mn=sn-abc-1":                 "r12---sn-abc-1.googlevideo.com",
		"fvip=5":                              "",
		"mn=sn-vgqskndl":                      "",
		"fvip=5&mn=evil.example.com%2F":       "",
		"fvip=5x&mn=sn-vgqskndl":              "",
	}
	for query, want := range cases {
		params, _ := url.ParseQuery(query)
		if got := hostFromServerParams(params); got != want {
			t.Errorf("%s: got %q, want %q", query, got, want)
		}
	}
}

type hostRecordingTransport struct{ hosts *[]string }

func (h hostRecordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	*h.hosts = append(*h.hosts, req.URL.Host)
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: http.Header{}, Request: req}, nil
}

func TestURLWithoutHostGoesToTheServerItsParamsName(t *testing.T) {
	config.LoadConfig()
	log.StartLogger()
	var hosts []string
	httpc.Client = &http.Client{Transport: hostRecordingTransport{&hosts}}

	target := "/videoplayback?itag=140&expire=9999999999&fvip=5&mn=sn-vgqskndl%2Csn-vgqsrnd7&ip=2602%3Afb54%3A1434%3A60ea%3A%3A1"
	rec := httptest.NewRecorder()
	Videoplayback(rec, httptest.NewRequest(http.MethodGet, target, nil))

	if rec.Code != http.StatusOK || len(hosts) != 1 || hosts[0] != "r5---sn-vgqskndl.googlevideo.com" {
		t.Fatalf("got status %d, upstream hosts %v", rec.Code, hosts)
	}
}
