package routes

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"git.nadeko.net/Fijxu/http3-ytproxy/internal/httpc"
	"git.nadeko.net/Fijxu/http3-ytproxy/internal/log"
	"git.nadeko.net/Fijxu/http3-ytproxy/internal/metrics"
	"git.nadeko.net/Fijxu/http3-ytproxy/internal/utils"
)

func Ggpht(w http.ResponseWriter, req *http.Request) {
	path := req.URL.EscapedPath()
	path = strings.Replace(path, "/ggpht", "", 1)
	path = strings.Replace(path, "/i/", "/", 1)

	proxyURL, err := url.Parse("https://" + constants.GgphtHost + path)
	if err != nil {
		log.Logger.Panic().Err(err).Msg("failed to create proxy URL for /ggpht request")
	}

	request, err := http.NewRequest(req.Method, proxyURL.String(), nil)
	if err != nil {
		log.Logger.Panic().Err(err).Msg("failed to create request")
	}

	utils.CopyHeaders(req.Header, request.Header, false)
	request.Header.Set("User-Agent", constants.DefaultUa)

	resp, err := httpc.Client.Do(request)
	if err != nil {
		log.Logger.Panic().Err(err).Msg("failed to do request")
	}

	if err := forbiddenChecker(resp, w); err != nil {
		metrics.Metrics.RequestForbidden.Ggpht.Inc()
		return
	}

	defer resp.Body.Close()

	NoRewrite := strings.HasPrefix(resp.Header.Get("Content-Type"), "audio") || strings.HasPrefix(resp.Header.Get("Content-Type"), "video")
	utils.CopyHeaders(resp.Header, w.Header(), NoRewrite)
	w.WriteHeader(resp.StatusCode)

	_, _ = io.Copy(w, resp.Body)
}
