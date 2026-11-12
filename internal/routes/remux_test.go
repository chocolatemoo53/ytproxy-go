package routes

import (
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"git.nadeko.net/Fijxu/ytproxy-go/internal/config"
)

func remuxTestRequest(params url.Values) *httptest.ResponseRecorder {
	config.LoadConfig()
	rec := httptest.NewRecorder()
	Remux(rec, httptest.NewRequest("GET", "/videoplayback/remux?"+params.Encode(), nil))
	return rec
}

func TestRemuxArgsUseLocalVideoplayback(t *testing.T) {
	params := url.Values{
		"f": {"webm"},
		"v": {"itag=248&host=rr1---sn-test.googlevideo.com&expire=1"},
		"a": {"itag=251&host=rr1---sn-test.googlevideo.com&expire=1"},
		"t": {"90"},
	}
	req := httptest.NewRequest("GET", "/videoplayback/remux?"+params.Encode(), nil)
	args, contentType, err := remuxArgs(req, "http://127.0.0.1:8080/videoplayback?")
	if err != nil || contentType != "video/webm" {
		t.Fatalf("got %q, %v", contentType, err)
	}
	inputs := 0
	for i, arg := range args {
		if arg == "-i" {
			inputs++
			if !strings.HasPrefix(args[i+1], "http://127.0.0.1:8080/videoplayback?") {
				t.Fatalf("input %q is not the local proxy", args[i+1])
			}
		}
	}
	if inputs != 2 || !slices.Contains(args, "90") || !slices.Contains(args, "copy") {
		t.Fatalf("unexpected args %v", args)
	}
}

func TestRemuxRejectsOtherHostsAndFormats(t *testing.T) {
	good := "itag=248&host=rr1---sn-test.googlevideo.com"
	for name, params := range map[string]url.Values{
		"foreign host":   {"f": {"webm"}, "v": {"itag=248&host=evil.example.com"}, "a": {good}},
		"missing itag":   {"f": {"webm"}, "v": {"host=rr1---sn-test.googlevideo.com"}, "a": {good}},
		"unknown format": {"f": {"mkv"}, "v": {good}, "a": {good}},
	} {
		if rec := remuxTestRequest(params); rec.Code != 400 {
			t.Errorf("%s: got %d, want 400", name, rec.Code)
		}
	}
}
