package metrics

import (
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// A playing player asks for a new piece of its streams every few seconds, so a
// stream that has been quiet for this long is no longer playing.
const playbackWindow = time.Minute

type stream struct{ session, itag string }

type streamSeen struct {
	at                time.Time
	kind, format, app string
}

type playback struct {
	mu        sync.Mutex
	seen      map[stream]streamSeen
	lastPrune time.Time
	sessions  *prometheus.Desc
	byApp     *prometheus.Desc
	streams   *prometheus.Desc
}

// Playback counts the playback sessions and their streams by format. A session
// is googlevideo's `ei`, which every player response gets anew, so nothing
// about the viewer is kept.
var Playback = &playback{
	seen: map[stream]streamSeen{},
	sessions: prometheus.NewDesc("http3_ytproxy_playback_sessions",
		"Playback sessions that fetched video or audio in the last minute", nil, nil),
	byApp: prometheus.NewDesc("http3_ytproxy_playback_sessions_by_app",
		"Playback sessions in the last minute, by the site the player runs on", []string{"app"}, nil),
	streams: prometheus.NewDesc("http3_ytproxy_playback_streams",
		"Streams fetched in the last minute, by kind and format", []string{"kind", "format"}, nil),
}

func init() {
	prometheus.MustRegister(Playback)
}

func (p *playback) Seen(session, itag, mime, app string) {
	p.seenAt(time.Now(), session, itag, mime, app)
}

func (p *playback) seenAt(now time.Time, session, itag, mime, app string) {
	if session == "" || itag == "" {
		return
	}
	kind, format := describe(itag, mime)
	p.mu.Lock()
	defer p.mu.Unlock()
	if now.Sub(p.lastPrune) > playbackWindow {
		p.prune(now)
	}
	p.seen[stream{session, itag}] = streamSeen{now, kind, format, app}
}

func (p *playback) prune(now time.Time) {
	for key, s := range p.seen {
		if now.Sub(s.at) > playbackWindow {
			delete(p.seen, key)
		}
	}
	p.lastPrune = now
}

func (p *playback) snapshot(now time.Time) (sessions int, streams map[[2]string]int, apps map[string]int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prune(now)
	ids := map[string]string{}
	streams = map[[2]string]int{}
	for key, s := range p.seen {
		ids[key.session] = s.app
		streams[[2]string{s.kind, s.format}]++
	}
	apps = map[string]int{}
	for _, app := range ids {
		apps[app]++
	}
	return len(ids), streams, apps
}

func (p *playback) Describe(ch chan<- *prometheus.Desc) {
	ch <- p.sessions
	ch <- p.byApp
	ch <- p.streams
}

func (p *playback) Collect(ch chan<- prometheus.Metric) {
	sessions, streams, apps := p.snapshot(time.Now())
	ch <- prometheus.MustNewConstMetric(p.sessions, prometheus.GaugeValue, float64(sessions))
	for app, n := range apps {
		ch <- prometheus.MustNewConstMetric(p.byApp, prometheus.GaugeValue, float64(n), app)
	}
	for labels, n := range streams {
		ch <- prometheus.MustNewConstMetric(p.streams, prometheus.GaugeValue, float64(n), labels[0], labels[1])
	}
}

var knownApps = []string{"chocolatemoo53.com", "pigeonduck.com", "ytify.pp.ua"}

func App(tagged, origin, referer string) string {
	if tagged != "" {
		return tagged
	}
	if origin == "" || origin == "null" {
		origin = referer
	}
	if origin == "" {
		return "none"
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return "other"
	}
	host := strings.ToLower(u.Hostname())
	for _, app := range knownApps {
		if host == app || strings.HasSuffix(host, "."+app) {
			return app
		}
	}
	if strings.HasSuffix(host, ".onion") || strings.HasSuffix(host, ".i2p") {
		return "chocolatemoo53.com"
	}
	return "other"
}

func FormatName(itag string) string {
	return itagFormats[itag][1]
}

func describe(itag, mime string) (kind, format string) {
	if f, ok := itagFormats[itag]; ok {
		return f[0], f[1]
	}
	kind, _, _ = strings.Cut(mime, "/")
	if kind != "video" && kind != "audio" {
		kind = "other"
	}
	if _, err := strconv.Atoi(itag); err != nil {
		itag = "?"
	}
	return kind, "itag " + itag
}

var itagFormats = map[string][2]string{
	"18": {"video", "360p H.264 + AAC"},

	"160": {"video", "144p H.264"},
	"133": {"video", "240p H.264"},
	"134": {"video", "360p H.264"},
	"135": {"video", "480p H.264"},
	"136": {"video", "720p H.264"},
	"137": {"video", "1080p H.264"},
	"298": {"video", "720p60 H.264"},
	"299": {"video", "1080p60 H.264"},
	"264": {"video", "1440p H.264"},
	"266": {"video", "2160p H.264"},

	"278": {"video", "144p VP9"},
	"242": {"video", "240p VP9"},
	"243": {"video", "360p VP9"},
	"244": {"video", "480p VP9"},
	"247": {"video", "720p VP9"},
	"248": {"video", "1080p VP9"},
	"271": {"video", "1440p VP9"},
	"313": {"video", "2160p VP9"},
	"302": {"video", "720p60 VP9"},
	"303": {"video", "1080p60 VP9"},
	"308": {"video", "1440p60 VP9"},
	"315": {"video", "2160p60 VP9"},
	"330": {"video", "144p60 VP9 HDR"},
	"331": {"video", "240p60 VP9 HDR"},
	"332": {"video", "360p60 VP9 HDR"},
	"333": {"video", "480p60 VP9 HDR"},
	"334": {"video", "720p60 VP9 HDR"},
	"335": {"video", "1080p60 VP9 HDR"},
	"336": {"video", "1440p60 VP9 HDR"},
	"337": {"video", "2160p60 VP9 HDR"},
	"356": {"video", "1080p VP9 Premium"},

	"394": {"video", "144p AV1"},
	"395": {"video", "240p AV1"},
	"396": {"video", "360p AV1"},
	"397": {"video", "480p AV1"},
	"398": {"video", "720p AV1"},
	"399": {"video", "1080p AV1"},
	"400": {"video", "1440p AV1"},
	"401": {"video", "2160p AV1"},
	"402": {"video", "4320p AV1"},
	"571": {"video", "4320p AV1"},
	"694": {"video", "144p AV1 HDR"},
	"695": {"video", "240p AV1 HDR"},
	"696": {"video", "360p AV1 HDR"},
	"697": {"video", "480p AV1 HDR"},
	"698": {"video", "720p AV1 HDR"},
	"699": {"video", "1080p AV1 HDR"},
	"700": {"video", "1440p AV1 HDR"},
	"701": {"video", "2160p AV1 HDR"},
	"702": {"video", "4320p AV1 HDR"},

	"599": {"audio", "AAC 30k"},
	"139": {"audio", "AAC 48k"},
	"140": {"audio", "AAC 128k"},
	"141": {"audio", "AAC 256k"},
	"256": {"audio", "AAC 5.1"},
	"258": {"audio", "AAC 5.1"},
	"327": {"audio", "AAC 5.1"},
	"600": {"audio", "Opus 35k"},
	"249": {"audio", "Opus 50k"},
	"250": {"audio", "Opus 70k"},
	"251": {"audio", "Opus 160k"},
}
