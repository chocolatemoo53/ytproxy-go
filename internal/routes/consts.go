package routes

import (
	"io"
	"net/http"
	"regexp"

	"git.nadeko.net/Fijxu/ytproxy-go/internal/utils"
)

var defaultUa = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36"

var constants = struct {
	DefaultUa        string
	GgphtHost        string
	AndroidUserAgent string
	WebUserAgent     string
	IosUserAgent     string
	TvUserAgent      string
}{
	DefaultUa:        defaultUa,
	GgphtHost:        "yt3.ggpht.com",
	AndroidUserAgent: "com.google.android.youtube/1537338816 (Linux; U; Android 13; en_US; ; Build/TQ2A.230505.002; Cronet/113.0.5672.24)",
	WebUserAgent:     defaultUa,
	IosUserAgent:     "com.google.ios.youtube/19.32.8 (iPhone14,5; U; CPU iOS 17_6 like Mac OS X;)",
	TvUserAgent:      "Mozilla/5.0 (SMART-TV; Linux; Tizen 4.0.0.2) AppleWebkit/605.1.15 (KHTML, like Gecko) SamsungBrowser/9.2 TV Safari/605.1.15",
}

func newProxyRequest(req *http.Request, proxyURL string, body io.Reader) (*http.Request, error) {
	request, err := http.NewRequest(req.Method, proxyURL, body)
	if err != nil {
		return nil, err
	}

	utils.CopyHeaders(req.Header, request.Header, false)
	request.Header.Set("User-Agent", constants.DefaultUa)
	return request, nil
}

var manifest_re = regexp.MustCompile(`(?m)URI="([^"]+)"`)

var allowed_hosts = []string{
	"youtube.com",
	"googlevideo.com",
	"ytimg.com",
	"ggpht.com",
	"googleusercontent.com",
}

var defaultHeaders = http.Header{
	"Accept":          {"*/*"},
	"Accept-Encoding": {"gzip, deflate, br, zstd"},
	"Accept-Language": {"en-us,en;q=0.5"},
	"Origin":          {"https://www.youtube.com"},
	"Referer":         {"https://www.youtube.com/"},
	"User-Agent":      {constants.DefaultUa},
}

// https://github.com/FreeTubeApp/FreeTube/blob/5a4cd981cdf2c2a20ab68b001746658fd0c6484e/src/renderer/components/ft-shaka-video-player/ft-shaka-video-player.js#L1097
var protobuf_body = []byte{0x78, 0} // protobuf body
