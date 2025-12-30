package routes

import (
	"net"
	"net/http"
	"strings"
	"time"

	"git.nadeko.net/Fijxu/http3-ytproxy/internal/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var programInit = time.Now()

// CustomHandler wraps the default promhttp.Handler with custom logic
func MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// To prevent accessing from the bare IP address
		if req.Host == "" || net.ParseIP(strings.Split(req.Host, ":")[0]) != nil {
			w.WriteHeader(444)
			return
		}

		metrics.Metrics.Uptime.Set(float64(time.Duration(time.Since(programInit).Seconds())))
		promhttp.Handler().ServeHTTP(w, req)
	})
}
