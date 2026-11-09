package routes

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"git.nadeko.net/Fijxu/ytproxy-go/internal/config"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/metrics"
)

const (
	videoCacheMaxRange   = 32 << 20
	videoCacheMinStart   = 1024
	videoCacheSweepEvery = 10 * time.Minute
	videoCacheSeenFor    = 24 * time.Hour
	videoCachePopular    = "popular"
	videoCacheTempPrefix = ".tmp-"
)

var (
	videoCacheDigits = regexp.MustCompile(`^[0-9]+$`)
	videoCacheMime   = regexp.MustCompile(`^(video|audio)/(mp4|webm)$`)
)

type videoCacheEntry struct {
	dir, path, mime, clen string
	from, to              int64
	popular               bool
}

func videoCacheFormat(itag string) bool {
	format := metrics.FormatName(itag)
	if format == "AAC 128k" {
		return true
	}
	resolution, _, _ := strings.Cut(format, "p")
	height, err := strconv.Atoi(resolution)
	return err == nil && height >= 1080
}

func newVideoCacheEntry(req *http.Request, q url.Values, host, title string, from, to int64) *videoCacheEntry {
	dir := config.Cfg.Video_cache_dir
	itag, lmt, clen, mime := q.Get("itag"), q.Get("lmt"), q.Get("clen"), q.Get("mime")
	signed := strings.Split(q.Get("sparams"), ",")
	switch {
	case dir == "", req.Method != http.MethodGet, req.Header.Get("Range") == "", title != "":
		return nil
	case !strings.HasSuffix(host, ".googlevideo.com"), !videoCacheFormat(itag), !videoCacheMime.MatchString(mime):
		return nil
	case !videoCacheDigits.MatchString(itag), !videoCacheDigits.MatchString(lmt), !videoCacheDigits.MatchString(clen):
		return nil
	case !slices.Contains(signed, "itag"), !slices.Contains(signed, "lmt"), !slices.Contains(signed, "clen"):
		return nil
	case to <= from, to-from+1 > videoCacheMaxRange:
		return nil
	}
	formatDir := filepath.Join(dir, itag+"_"+lmt+"_"+clen)
	name := strconv.FormatInt(from, 10) + "-" + strconv.FormatInt(to, 10)
	return &videoCacheEntry{formatDir, filepath.Join(formatDir, name), mime, clen, from, to, false}
}

func (e *videoCacheEntry) serve(w http.ResponseWriter) bool {
	file, err := os.Open(e.path)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() != e.to-e.from+1 {
		return false
	}
	now := time.Now()
	_ = os.Chtimes(e.dir, now, now)
	header := w.Header()
	header.Set("Accept-Ranges", "bytes")
	header.Set("Content-Type", e.mime)
	header.Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	header.Set("Content-Range", "bytes "+strconv.FormatInt(e.from, 10)+"-"+strconv.FormatInt(e.to, 10)+"/"+e.clen)
	w.WriteHeader(http.StatusPartialContent)
	written, _ := io.Copy(w, file)
	metrics.Metrics.VideoCacheBytes.WithLabelValues("hit").Add(float64(written))
	return true
}

func (e *videoCacheEntry) admit() {
	popular := filepath.Join(e.dir, videoCachePopular)
	if e.from == 0 && e.to >= videoCacheMinStart {
		if err := os.Mkdir(e.dir, 0o755); errors.Is(err, fs.ErrExist) || (err == nil && config.Cfg.Video_cache_first_view) {
			if file, err := os.OpenFile(popular, os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
				file.Close()
			}
		}
	}
	_, err := os.Stat(popular)
	e.popular = err == nil
}

type videoCacheWriter struct {
	entry   *videoCacheEntry
	w       io.Writer
	temp    *os.File
	written int64
}

func (e *videoCacheEntry) writer(w io.Writer) *videoCacheWriter {
	writer := &videoCacheWriter{entry: e, w: w}
	if e.popular {
		writer.temp, _ = os.CreateTemp(e.dir, videoCacheTempPrefix+"*")
	}
	return writer
}

func (c *videoCacheWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.written += int64(n)
	if c.temp != nil {
		if _, tempErr := c.temp.Write(p[:n]); tempErr != nil {
			c.discard()
		}
	}
	return n, err
}

func (c *videoCacheWriter) discard() {
	c.temp.Close()
	os.Remove(c.temp.Name())
	c.temp = nil
}

func (c *videoCacheWriter) finish() {
	metrics.Metrics.VideoCacheBytes.WithLabelValues("miss").Add(float64(c.written))
	if c.temp == nil {
		return
	}
	if c.written != c.entry.to-c.entry.from+1 {
		c.discard()
		return
	}
	name := c.temp.Name()
	c.temp.Close()
	if os.Chmod(name, 0o644) != nil || os.Rename(name, c.entry.path) != nil {
		os.Remove(name)
	}
}

func StartVideoCacheSweeper() {
	dir := config.Cfg.Video_cache_dir
	if dir == "" {
		return
	}
	go func() {
		for {
			sweepVideoCache(dir, config.Cfg.Video_cache_gb<<30, time.Now())
			time.Sleep(videoCacheSweepEvery)
		}
	}()
}

func sweepVideoCache(dir string, maxBytes int64, now time.Time) {
	type cachedFormat struct {
		path     string
		size     int64
		modified time.Time
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var formats []cachedFormat
	var total int64
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		files, readErr := os.ReadDir(path)
		if err != nil || readErr != nil {
			continue
		}
		format := cachedFormat{path: path, modified: info.ModTime()}
		stored := false
		for _, file := range files {
			fileInfo, err := file.Info()
			switch {
			case err != nil, file.Name() == videoCachePopular:
			case strings.HasPrefix(file.Name(), videoCacheTempPrefix):
				if now.Sub(fileInfo.ModTime()) > videoCacheSweepEvery {
					os.Remove(filepath.Join(path, file.Name()))
				}
			default:
				stored = true
				format.size += fileInfo.Size()
			}
		}
		if !stored {
			if now.Sub(format.modified) > videoCacheSeenFor {
				os.RemoveAll(path)
			}
			continue
		}
		formats = append(formats, format)
		total += format.size
	}
	sort.Slice(formats, func(i, j int) bool { return formats[i].modified.Before(formats[j].modified) })
	for _, format := range formats {
		if total <= maxBytes {
			return
		}
		if os.RemoveAll(format.path) == nil {
			total -= format.size
		}
	}
}
