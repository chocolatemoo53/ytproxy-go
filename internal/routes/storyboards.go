package routes

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	"git.nadeko.net/Fijxu/ytproxy-go/internal/httpc"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/log"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/metrics"
)

func Storyboards(w http.ResponseWriter, req *http.Request) {
	authority := req.PathValue("authority")
	id := req.PathValue("id")
	storyboard := req.PathValue("storyboard")
	index := req.PathValue("index")

	if authority == "" || id == "" || storyboard == "" || index == "" {
		http.Error(w, "missing path parameters", http.StatusBadRequest)
		return
	}

	q := req.URL.Query()

	proxyURL, err := url.Parse(fmt.Sprintf("https://%s.ytimg.com/sb/%s/%s/%s", authority, id, storyboard, index))
	if err != nil {
		log.Logger.Panic().Err(err).Msg("failed to create proxy URL for /sb request")
	}

	/*
		Required for /sb/ endpoints
		You can't access https://i.ytimg.com/sb/<VIDEOID>/storyboard3_L2/M3.jpg
		without it's parameters `sqp` and `sigh`
	*/
	proxyURL.RawQuery = q.Encode()

	request, err := newProxyRequest(req, proxyURL.String(), nil)
	if err != nil {
		log.Logger.Panic().Err(err).Msg("failed to create request")
	}

	resp, err := httpc.Client.Do(request)
	if err != nil {
		log.Logger.Panic().Err(err).Msg("failed to do request")
	}

	if err := forbiddenChecker(resp, w); err != nil {
		metrics.Metrics.RequestForbidden.Storyboards.Inc()
		return
	}

	defer resp.Body.Close()

	w.WriteHeader(resp.StatusCode)

	_, _ = io.Copy(w, resp.Body)
}
