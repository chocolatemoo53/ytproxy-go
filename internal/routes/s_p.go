package routes

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"git.nadeko.net/Fijxu/ytproxy-go/internal/httpc"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/log"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/metrics"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/utils"
)

func S_p(authority string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query()
		path := req.URL.EscapedPath()

		proxyURL, err := url.Parse("https://" + authority + ".ytimg.com" + path)
		if err != nil {
			log.Logger.Panic().Err(err).Msg("failed to create proxy URL for routes using S_p()")
		}

		if encodedQuery := q.Encode(); encodedQuery == "" {
			proxyURL.RawQuery = encodedQuery
		}

		request, err := newProxyRequest(req, proxyURL.String(), nil)
		if err != nil {
			log.Logger.Panic().Err(err).Msg("failed to create request")
		}

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
}
