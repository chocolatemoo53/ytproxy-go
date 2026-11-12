package routes

import (
	"errors"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"git.nadeko.net/Fijxu/ytproxy-go/internal/config"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/log"
)

const remuxMaxStart = 24 * 60 * 60

var remuxSlots = make(chan struct{}, 32)

var remuxFormats = map[string][]string{
	"webm": {"-f", "webm"},
	"mp4":  {"-f", "mp4", "-movflags", "frag_keyframe+empty_moov+default_base_moof"},
}

func remuxInput(raw string) (string, error) {
	q, err := url.ParseQuery(raw)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(q.Get("host"), ".googlevideo.com") || q.Get("itag") == "" {
		return "", errors.New("not a googlevideo stream")
	}
	return q.Encode(), nil
}

func remuxArgs(req *http.Request, local string) ([]string, string, error) {
	q := req.URL.Query()
	format, ok := remuxFormats[q.Get("f")]
	if !ok {
		return nil, "", errors.New("unknown format")
	}
	video, err := remuxInput(q.Get("v"))
	if err != nil {
		return nil, "", err
	}
	audio, err := remuxInput(q.Get("a"))
	if err != nil {
		return nil, "", err
	}
	start, _ := strconv.Atoi(q.Get("t"))
	start = max(0, min(start, remuxMaxStart))
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	for _, input := range []string{video, audio} {
		if start > 0 {
			args = append(args, "-ss", strconv.Itoa(start))
		}
		args = append(args, "-readrate", "1.5", "-readrate_initial_burst", "20", "-i", local+input)
	}
	args = append(args, "-map", "0:v:0", "-map", "1:a:0", "-c", "copy")
	args = append(args, format...)
	return append(args, "pipe:1"), "video/" + q.Get("f"), nil
}

type flushWriter struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

func (f flushWriter) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	_ = f.rc.Flush()
	return n, err
}

func Remux(w http.ResponseWriter, req *http.Request) {
	local := "http://127.0.0.1:" + config.Cfg.Server.Port + "/videoplayback?"
	args, contentType, err := remuxArgs(req, local)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	select {
	case remuxSlots <- struct{}{}:
		defer func() { <-remuxSlots }()
	default:
		http.Error(w, "too many remuxes", http.StatusServiceUnavailable)
		return
	}
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	cmd := exec.CommandContext(req.Context(), "ffmpeg", args...)
	cmd.Stdout = flushWriter{w, rc}
	if err := cmd.Run(); err != nil && req.Context().Err() == nil {
		log.Logger.Warn().Err(err).Msg("remux ended early")
	}
}
