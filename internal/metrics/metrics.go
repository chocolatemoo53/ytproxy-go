package metrics

import "github.com/prometheus/client_golang/prometheus"

type metrics struct {
	Uptime                 prometheus.Gauge
	RequestCount           prometheus.Counter
	RequestPerSecond       prometheus.Gauge
	RequestPerMinute       prometheus.Gauge
	TotalConnEstablished   prometheus.Counter
	EstablishedConnections prometheus.Gauge
	ActiveConnections      prometheus.Gauge
	IdleConnections        prometheus.Gauge
	VideoCacheBytes        *prometheus.CounterVec
	VideoplaybackBytes     *prometheus.CounterVec
	RequestForbidden       struct {
		Videoplayback prometheus.Counter
		Vi            prometheus.Counter
		Storyboards   prometheus.Counter
		Ggpht         prometheus.Counter
	}
}

var Metrics = metrics{
	Uptime: prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "http3_ytproxy_uptime",
	}),
	RequestCount: prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "http3_ytproxy_request_count",
	}),
	RequestPerSecond: prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "http3_ytproxy_request_per_second",
	}),
	RequestPerMinute: prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "http3_ytproxy_request_per_minute",
	}),
	TotalConnEstablished: prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "http3_ytproxy_total_conn_established",
	}),
	EstablishedConnections: prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "http3_ytproxy_established_conns",
	}),
	ActiveConnections: prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "http3_ytproxy_active_conns",
	}),
	IdleConnections: prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "http3_ytproxy_idle_conns",
	}),
	VideoCacheBytes: prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http3_ytproxy_video_cache_bytes",
		Help: "Bytes of cacheable formats sent to viewers, by whether they came from disk or googlevideo",
	}, []string{"result"}),
	VideoplaybackBytes: prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http3_ytproxy_videoplayback_bytes",
		Help: "Video bytes read from googlevideo and written to viewers",
	}, []string{"direction"}),

	RequestForbidden: struct {
		Videoplayback prometheus.Counter
		Vi            prometheus.Counter
		Storyboards   prometheus.Counter
		Ggpht         prometheus.Counter
	}{
		Videoplayback: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "http3_ytproxy_request_forbidden_videoplayback",
		}),
		Vi: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "http3_ytproxy_request_forbidden_vi",
		}),
		Storyboards: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "http3_ytproxy_request_forbidden_storyboards",
		}),
		Ggpht: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "http3_ytproxy_request_forbidden_ggpht",
		}),
	},
}

func init() {
	prometheus.MustRegister(Metrics.Uptime)
	prometheus.MustRegister(Metrics.ActiveConnections)
	prometheus.MustRegister(Metrics.IdleConnections)
	prometheus.MustRegister(Metrics.EstablishedConnections)
	prometheus.MustRegister(Metrics.TotalConnEstablished)
	prometheus.MustRegister(Metrics.RequestCount)
	prometheus.MustRegister(Metrics.RequestPerSecond)
	prometheus.MustRegister(Metrics.RequestPerMinute)
	prometheus.MustRegister(Metrics.VideoCacheBytes)
	prometheus.MustRegister(Metrics.VideoplaybackBytes)
	prometheus.MustRegister(Metrics.RequestForbidden.Videoplayback)
	prometheus.MustRegister(Metrics.RequestForbidden.Vi)
	prometheus.MustRegister(Metrics.RequestForbidden.Storyboards)
	prometheus.MustRegister(Metrics.RequestForbidden.Ggpht)
}
