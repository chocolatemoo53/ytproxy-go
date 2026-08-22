package routes

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	"git.nadeko.net/Fijxu/ytproxy-go/internal/httpc"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/log"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/metrics"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/utils"
)

func Vi(w http.ResponseWriter, req *http.Request) {
	const host string = "i.ytimg.com"
	id := req.PathValue("id")
	name := req.PathValue("name")

	proxyURL, err := url.Parse(fmt.Sprintf("https://%s/vi/%s/%s", host, id, name))
	if err != nil {
		log.Logger.Panic().Err(err).Msg("failed to create proxy URL for /vi request")
	}

	if name == "maxres.jpg" {
		proxyURL.Path = utils.GetBestThumbnail(proxyURL.EscapedPath())
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
		metrics.Metrics.RequestForbidden.Vi.Inc()
		return
	}

	defer resp.Body.Close()

	w.WriteHeader(resp.StatusCode)

	_, _ = io.Copy(w, resp.Body)
}
