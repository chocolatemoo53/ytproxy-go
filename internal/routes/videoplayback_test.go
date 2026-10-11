package routes

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"git.nadeko.net/Fijxu/ytproxy-go/internal/config"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/httpc"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/log"
)

type forbiddenTransport struct{}

func (forbiddenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
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
	httpc.Client = &http.Client{Transport: forbiddenTransport{}}

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

	t.Setenv("YTPROXY_COMPANION_FALLBACK_PATH", "")
	config.LoadConfig()
	rec = httptest.NewRecorder()
	Videoplayback(rec, httptest.NewRequest("GET", target, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("without a fallback the 403 is passed on, got %d", rec.Code)
	}
}
