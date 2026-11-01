package routes

import (
	"io"
	"net/http"
	"regexp"

	"git.nadeko.net/Fijxu/ytproxy-go/internal/utils"
)

var defaultUa = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36"

// ponytail: copies of the companion's INNERTUBE_CLIENTS user agents (youtubePlayerReq.ts), update both together
var constants = struct {
	DefaultUa          string
	GgphtHost          string
	VisionosUserAgent  string
	AndroidVrUserAgent string
	TvUserAgent        string
	TvSamsungUserAgent string
}{
	DefaultUa:          defaultUa,
	GgphtHost:          "yt3.ggpht.com",
	VisionosUserAgent:  "com.google.visionos.youtube/1.04 (iPhone; U; iOS 26.6.0; en_US) C4D5D05E-F7B8-44F7-AB95-B13A1B8E4B41",
	AndroidVrUserAgent: "com.google.android.apps.youtube.vr.oculus/1.65.10 (Linux; U; Android 12L; eureka-user Build/SQ3A.220605.009.A1) gzip",
	TvUserAgent:        "Mozilla/5.0 (ChromiumStylePlatform) Cobalt/25.lts.30.1034943-gold (unlike Gecko), Unknown_TV_Unknown_0/Unknown (Unknown, Unknown)",
	TvSamsungUserAgent: "Mozilla/5.0 (SMART-TV; Linux; Tizen 2.4.0) AppleWebKit/538.1 (KHTML, like Gecko) Version/2.4.0 TV Safari/538.1",
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
