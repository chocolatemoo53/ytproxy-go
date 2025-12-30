package routes

import (
	"net/http"
	"regexp"
)

const (
	default_ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36"
	ggpht_host = "yt3.ggpht.com"
)

var manifest_re = regexp.MustCompile(`(?m)URI="([^"]+)"`)

var allowed_hosts = []string{
	"youtube.com",
	"googlevideo.com",
	"ytimg.com",
	"ggpht.com",
	"googleusercontent.com",
}

var videoplayback_headers = &http.Header{
	"Accept":          {"*/*"},
	"Accept-Encoding": {"gzip, deflate, br, zstd"},
	"Accept-Language": {"en-us,en;q=0.5"},
	"Origin":          {"https://www.youtube.com"},
	"Referer":         {"https://www.youtube.com/"},
	"User-Agent":      {default_ua},
}

// https://github.com/FreeTubeApp/FreeTube/blob/5a4cd981cdf2c2a20ab68b001746658fd0c6484e/src/renderer/components/ft-shaka-video-player/ft-shaka-video-player.js#L1097
var protobuf_body = []byte{0x78, 0} // protobuf body
