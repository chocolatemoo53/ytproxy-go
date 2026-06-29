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

func Vi(w http.ResponseWriter, req *http.Request) {
	const host string = "i.ytimg.com"
	q := req.URL.Query()
	path := req.URL.EscapedPath()

	proxyURL, err := url.Parse("https://" + host + path)
	if err != nil {
		log.Logger.Panic().Err(err).Msg("failed to create proxy URL for /vi request")
	}

	if strings.HasSuffix(proxyURL.EscapedPath(), "maxres.jpg") {
		proxyURL.Path = utils.GetBestThumbnail(proxyURL.EscapedPath())
	}

	/*
		Required for /sb/ endpoints
		You can't access https://i.ytimg.com/sb/<VIDEOID>/storyboard3_L2/M3.jpg
		without it's parameters `sqp` and `sigh`
	*/
	proxyURL.RawQuery = q.Encode()

	request, err := http.NewRequest(req.Method, proxyURL.String(), nil)
	if err != nil {
		log.Logger.Panic().Err(err).Msg("failed to create request")
	}

	request.Header.Set("User-Agent", constants.DefaultUa)

	resp, err := httpc.Client.Do(request)
	if err != nil {
		log.Logger.Panic().Err(err).Msg("failed to do request")
	}

	if err := forbiddenChecker(resp, w); err != nil {
		metrics.Metrics.RequestForbidden.Vi.Inc()
		return
	}

	defer resp.Body.Close()

	w.WriteHeader(resp.StatusCode)

	_, _ = io.Copy(w, resp.Body)
}
