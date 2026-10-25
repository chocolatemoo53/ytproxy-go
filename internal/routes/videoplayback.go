package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.nadeko.net/Fijxu/ytproxy-go/internal/config"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/httpc"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/log"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/metrics"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/utils"
)

// Sessions whose URL googlevideo refused lately. Their requests are handed over
// to the companion straight away instead of being refused again first; the
// companion's fresh URLs carry a new session name, so they don't match.
var (
	refusedMu       sync.Mutex
	refusedSessions = map[string]time.Time{}
)

const refusedFor = 10 * time.Minute

func refusedRecently(session string) bool {
	refusedMu.Lock()
	defer refusedMu.Unlock()
	at, ok := refusedSessions[session]
	return ok && time.Since(at) < refusedFor
}

func markRefused(session string) {
	refusedMu.Lock()
	defer refusedMu.Unlock()
	if len(refusedSessions) > 5000 {
		for key, at := range refusedSessions {
			if time.Since(at) >= refusedFor {
				delete(refusedSessions, key)
			}
		}
	}
	refusedSessions[session] = time.Now()
}

func forbiddenChecker(resp *http.Response, w http.ResponseWriter) error {
	if resp.StatusCode == 403 {
		w.WriteHeader(403)
		responseText := `Forbidden 403
		Maybe Youtube blocked the IP of this proxy?`
		_, _ = io.WriteString(w, responseText)
		return fmt.Errorf("%s returned %d", resp.Request.Host, resp.StatusCode)
	}
	return nil
}

// hostFromServerParams is the googlevideo server a URL's fvip and mn params
// name ("r5---sn-abc.googlevideo.com"), or "" when they don't.
func hostFromServerParams(params url.Values) string {
	fvip := params.Get("fvip")
	mn, _, _ := strings.Cut(params.Get("mn"), ",")
	if !serverNumber.MatchString(fvip) || !serverName.MatchString(mn) {
		return ""
	}
	return "r" + fvip + "---" + mn + ".googlevideo.com"
}

var (
	serverNumber = regexp.MustCompile(`^[0-9]{1,2}$`)
	serverName   = regexp.MustCompile(`^sn-[a-z0-9-]+$`)
)

func checkRequest(w http.ResponseWriter, req *http.Request, params url.Values) bool {
	host := params.Get("host")

	parts := strings.Split(strings.ToLower(host), ".")
	if len(parts) < 2 {
		w.WriteHeader(400)
		_, _ = io.WriteString(w, "Invalid hostname.")
		return true
	}

	domain := parts[len(parts)-2] + "." + parts[len(parts)-1]
	disallowed := true
	for _, value := range allowed_hosts {
		if domain == value {
			disallowed = false
			break
		}
	}

	if disallowed {
		w.WriteHeader(401)
		_, _ = io.WriteString(w, "Non YouTube domains are not supported.")
		return true
	}

	expire, err := strconv.ParseInt(params.Get("expire"), 10, 64)
	if err != nil {
		w.WriteHeader(500)
		_, _ = io.WriteString(w, "Expire query string undefined")
		return true
	}

	// Prevent the process of already expired playbacks
	// since they will return 403 from googlevideo server
	if (expire - time.Now().Unix()) <= 0 {
		w.WriteHeader(403)
		_, _ = io.WriteString(w, "Videoplayback URL has expired.")
		return true
	}

	return false
}

// Send the user agent of the client the URL was issued to, named by its c and cver params
func setUserAgent(headers *http.Header, youtubeClient string, clientVersion string) {
	switch {
	// Native apps aren't on a youtube.com page, so they send no Origin or Referer
	case youtubeClient == "VISIONOS":
		headers.Set("User-Agent", constants.VisionosUserAgent)
		headers.Del("Origin")
		headers.Del("Referer")
	case youtubeClient == "ANDROID_VR":
		headers.Set("User-Agent", constants.AndroidVrUserAgent)
		headers.Del("Origin")
		headers.Del("Referer")
	case youtubeClient == "TVHTML5" && strings.HasPrefix(clientVersion, "5."):
		headers.Set("User-Agent", constants.TvSamsungUserAgent)
	case youtubeClient == "TVHTML5":
		headers.Set("User-Agent", constants.TvUserAgent)
	}
}

func processDecryptedQueryParams(structuredDecryptedQueryParams *[][]string, q *url.Values) {
	log.Logger.Debug().Msg("processing decrypted query parameters")
	for _, value := range *structuredDecryptedQueryParams {
		paramName := value[0]
		paramValue := value[1]
		log.Logger.Trace().Str(paramName, paramValue).Msg("setting decrypted query parameters")
		q.Set(value[0], value[1])
	}
}

func Videoplayback(w http.ResponseWriter, req *http.Request) {
	start := time.Now()
	q := req.URL.Query()

	if q.Get("enc") == "true" {
		encryptedData := q.Get("data")
		decryptedQueryParams, err := utils.DecryptQueryParams(encryptedData, config.Cfg.Companion.Secret_key)
		if err != nil {
			msg := "failed to decrypt query parameters"
			log.Logger.Error().Err(err).Msg(msg)
			http.Error(w, msg, http.StatusInternalServerError)
			return
		} else {
			log.Logger.Trace().Str("decryptedQueryParams", decryptedQueryParams).Msg("successfully decrypted the encrypted query parameters")
		}

		var structuredDecryptedQueryParams [][]string

		err = json.Unmarshal([]byte(decryptedQueryParams), &structuredDecryptedQueryParams)
		if err != nil {
			msg := "failed to parse decrypted query parameters"
			log.Logger.Error().Err(err).Msg(msg)
			http.Error(w, msg, http.StatusInternalServerError)
			return
		} else {
			log.Logger.Trace().Any("structuredDecryptedQueryParams", &structuredDecryptedQueryParams).Msg("successfully parsed decrypted query parameters")
		}

		q.Del("enc")
		q.Del("data")

		processDecryptedQueryParams(&structuredDecryptedQueryParams, &q)
	}

	// Apps that put the instance in place of googlevideo's host send no host
	// param; Invidious named the server from fvip and mn then
	if q.Get("host") == "" {
		q.Set("host", hostFromServerParams(q))
	}

	if checkRequest(w, req, q) {
		log.Logger.Debug().Msg("request didn't met required checks, closing")
		return
	}

	host := q.Get("host")
	title := q.Get("title")
	q.Del("host")
	q.Del("title")

	// The YouTube video id (set by the companion), only for the proxy: it
	// names the session, so everything for one video leaves from one address.
	session := q.Get("ytid")
	q.Del("ytid")
	dialSession := session
	if dialSession == "" {
		dialSession = httpc.AddressSession(q.Get("ip"))
	}
	ctx := httpc.WithSession(req.Context(), dialSession)

	playbackSession := q.Get("ei")
	if playbackSession == "" {
		playbackSession = session
	}
	metrics.Playback.Seen(playbackSession, q.Get("itag"), q.Get("mime"))

	// The companion's own /videoplayback asks YouTube for a fresh URL when one
	// is refused (another client, another address); a player sent here skips
	// that, so a refused request is handed over. Its `ytid` is the bare video id.
	canHandOver := config.Cfg.Companion.Fallback_path != "" && session != ""
	handOver := func() {
		fallback := req.URL.Query()
		fallback.Set("ytid", strings.SplitN(session, ".", 2)[0])
		http.Redirect(w, req, config.Cfg.Companion.Fallback_path+"/videoplayback?"+fallback.Encode(), http.StatusTemporaryRedirect)
	}
	if canHandOver && refusedRecently(session) {
		handOver()
		return
	}

	rangeHeader := req.Header.Get("range")
	var requestBytes string
	if rangeHeader != "" {
		requestBytes = strings.Split(rangeHeader, "=")[1]
	} else {
		requestBytes = ""
	}
	// googlevideo paces a long request to about the format's bitrate after a fast
	// start, so a 4K segment arrived as slowly as it plays: fetch it in pieces
	chunkFrom, chunkTo, chunked := chunkedRange(requestBytes)
	if chunked {
		q.Set("range", strconv.FormatInt(chunkFrom, 10)+"-"+strconv.FormatInt(chunkFrom+chunkSize-1, 10))
	} else if requestBytes != "" {
		q.Set("range", requestBytes)
	}

	path := req.URL.EscapedPath()

	proxyURL, err := url.Parse("https://" + host + path)
	if err != nil {
		log.Logger.Error().Err(err).Msg("failed to create proxy URL for /videoplayback request")
		http.Error(w, "failed to create proxy URL", http.StatusInternalServerError)
		return
	}

	proxyURL.RawQuery = q.Encode()

	postRequest, err := http.NewRequestWithContext(ctx, "GET", proxyURL.String(), nil)
	if err != nil {
		log.Logger.Error().Err(err).Msg("failed to create GET request")
		http.Error(w, "failed to create GET request", http.StatusInternalServerError)
		return
	}
	headRequest, err := http.NewRequestWithContext(ctx, "HEAD", proxyURL.String(), nil)
	if err != nil {
		log.Logger.Error().Err(err).Msg("failed to create HEAD request")
		http.Error(w, "failed to create HEAD request", http.StatusInternalServerError)
		return
	}

	youtubeClient := q.Get("c")

	videoplaybackHeaders := defaultHeaders.Clone()
	log.Logger.Trace().Any("headers", videoplaybackHeaders).Msg("setting default headers for /videoplayback request")
	setUserAgent(&videoplaybackHeaders, youtubeClient, q.Get("cver"))
	postRequest.Header = videoplaybackHeaders
	headRequest.Header = videoplaybackHeaders

	resp := &http.Response{}

	// https://datatracker.ietf.org/doc/html/draft-ietf-httpbis-p2-semantics-17#section-7.3
	// A maximum of 5 redirections is defined in the note of the section 7.3
	// of this RFC, that's why `i < 5`
	//
	// Only for clients that don't follow redirects themselves: the HTTP/3
	// client does, and the HEAD would cost every request a round trip.
	for i := 0; i < 5 && httpc.Client.CheckRedirect != nil; i++ {
		resp, err = httpc.Client.Do(headRequest)
		if err != nil {
			log.Logger.Error().Err(err).Str("session", session).Dur("after", time.Since(start)).Msg("failed to do HEAD request")
			http.Error(w, "failed to do HEAD request to googlevideo", http.StatusBadGateway)
			return
		}
		resp.Body.Close()
		if resp.Header.Get("Location") != "" {
			new_url, _ := url.Parse(resp.Header.Get("Location"))
			postRequest.URL = new_url
			headRequest.URL = new_url
			postRequest.Host = new_url.Host
			headRequest.Host = new_url.Host
			continue
		} else {
			break
		}
	}

	resp, err = httpc.Client.Do(postRequest)
	if err != nil {
		log.Logger.Error().Err(err).Str("session", session).Dur("after", time.Since(start)).Msg("failed to do GET request")
		http.Error(w, "failed to do GET request to googlevideo", http.StatusBadGateway)
		return
	}
	// Before the 403 check: an unclosed body holds its HTTP/3 stream
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden && canHandOver {
		metrics.Metrics.RequestForbidden.Videoplayback.Inc()
		log.Logger.Warn().Str("session", session).Str("host", resp.Request.Host).Dur("after", time.Since(start)).Msg("googlevideo answered 403, handing over to the companion")
		markRefused(session)
		handOver()
		return
	}

	if err := forbiddenChecker(resp, w); err != nil {
		metrics.Metrics.RequestForbidden.Videoplayback.Inc()
		log.Logger.Warn().Str("session", session).Str("host", resp.Request.Host).Dur("after", time.Since(start)).Msg("googlevideo answered 403")
		return
	}

	utils.CopyHeadersNew(resp.Header, w.Header())
	chunked = chunked && resp.StatusCode == http.StatusOK
	if chunked {
		w.Header().Set("Content-Length", strconv.FormatInt(chunkTo-chunkFrom+1, 10))
	}

	if title != "" {
		content := "attachment; filename=\"" + url.PathEscape(title) + "\"; filename*=UTF-8''" + url.PathEscape(title)
		w.Header().Set("content-disposition", content)
	}

	if requestBytes != "" && resp.StatusCode == http.StatusOK {
		// check for range headers in the forms:
		// "bytes=0-" get full length from start
		// "bytes=500-" get full length from 500 bytes in
		// "bytes=500-1000" get 500 bytes starting from 500
		byteParts := strings.Split(requestBytes, "-")
		firstByte, lastByte := byteParts[0], byteParts[1]
		if lastByte != "" {
			clen := q.Get("clen")

			if clen == "" {
				w.Header().Add("content-range", "bytes "+requestBytes+"/*")
			} else {
				w.Header().Add("content-range", "bytes "+requestBytes+"/"+clen)
			}

			w.WriteHeader(206)
		} else {
			// i.e. "bytes=0-", "bytes=600-"
			// full size of content is able to be calculated, so a full Content-Range header can be constructed
			bytesReceived := resp.Header.Get("content-length")
			firstByteInt, _ := strconv.Atoi(firstByte)
			bytesReceivedInt, _ := strconv.Atoi(bytesReceived)
			// last byte should always be one less than the length
			totalContentLength := firstByteInt + bytesReceivedInt
			lastByte := totalContentLength - 1
			lastByteString := strconv.Itoa(lastByte)
			totalContentLengthString := strconv.Itoa(totalContentLength)
			w.Header().Add("content-range", "bytes "+firstByte+"-"+lastByteString+"/"+totalContentLengthString)
			if firstByte != "0" {
				// only part of the total content returned, 206
				w.WriteHeader(206)
			}
		}
	}

	if req.Method == "GET" && (resp.Header.Get("Content-Type") == "application/x-mpegurl" || resp.Header.Get("Content-Type") == "application/vnd.apple.mpegurl") {
		bytes, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Logger.Error().Err(err).Msg("failed to read body")
			http.Error(w, "failed to read response body", http.StatusBadGateway)
			return
		}

		lines := strings.Split(string(bytes), "\n")
		reqUrl := resp.Request.URL
		for i := 0; i < len(lines); i++ {
			line := lines[i]
			if !strings.HasPrefix(line, "https://") && (strings.HasSuffix(line, ".m3u8") || strings.HasSuffix(line, ".ts")) {
				path := reqUrl.EscapedPath()
				path = path[0 : strings.LastIndex(path, "/")+1]
				line = "https://" + reqUrl.Hostname() + path + line
			}
			if strings.HasPrefix(line, "https://") {
				lines[i] = utils.RelativeUrl(line)
			}

			if manifest_re.MatchString(line) {
				url := manifest_re.FindStringSubmatch(line)[1]
				lines[i] = strings.Replace(line, url, utils.RelativeUrl(url), 1)
			}
		}

		_, _ = io.WriteString(w, strings.Join(lines, "\n"))
	} else {
		_, _ = io.Copy(w, resp.Body)
		if chunked {
			err := copyChunks(ctx, w, resp.Request.URL, postRequest.Header, chunkFrom+chunkSize, chunkTo)
			if err != nil {
				log.Logger.Warn().Err(err).Str("session", session).Msg("failed to fetch the rest of a chunked range")
			}
		}
	}
}

const (
	chunkSize   = 2 << 20
	chunksAhead = 4
)

// chunkedRange reads a closed range "from-to" longer than one chunk.
func chunkedRange(requestBytes string) (from, to int64, ok bool) {
	parts := strings.Split(requestBytes, "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	from, err1 := strconv.ParseInt(parts[0], 10, 64)
	to, err2 := strconv.ParseInt(parts[1], 10, 64)
	return from, to, err1 == nil && err2 == nil && to-from+1 > chunkSize
}

// copyChunks writes bytes from-to of the URL, fetched chunkSize at a time with up
// to chunksAhead requests in flight, in order.
func copyChunks(ctx context.Context, w io.Writer, base *url.URL, header http.Header, from, to int64) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		body []byte
		err  error
	}
	ahead := make(chan struct{}, chunksAhead)
	queue := make(chan chan result, chunksAhead)
	go func() {
		defer close(queue)
		for start := from; start <= to; start += chunkSize {
			select {
			case ahead <- struct{}{}:
			case <-ctx.Done():
				return
			}
			done := make(chan result, 1)
			queue <- done
			go func(start, end int64) {
				body, err := fetchChunk(ctx, base, header, start, end)
				done <- result{body, err}
			}(start, min(start+chunkSize-1, to))
		}
	}()
	for done := range queue {
		r := <-done
		<-ahead
		if r.err != nil {
			return r.err
		}
		if _, err := w.Write(r.body); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func fetchChunk(ctx context.Context, base *url.URL, header http.Header, start, end int64) ([]byte, error) {
	u := *base
	q := u.Query()
	q.Set("range", strconv.FormatInt(start, 10)+"-"+strconv.FormatInt(end, 10))
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header = header
	resp, err := httpc.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("googlevideo answered %d for bytes %d-%d", resp.StatusCode, start, end)
	}
	body, err := io.ReadAll(resp.Body)
	if err == nil && int64(len(body)) != end-start+1 {
		err = fmt.Errorf("got %d bytes for %d-%d", len(body), start, end)
	}
	return body, err
}
