package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"syscall"
	"time"

	"git.nadeko.net/Fijxu/ytproxy-go/internal/config"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/httpc"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/log"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/metrics"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/routes"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/utils"
)

type ConnectionWatcher struct {
	totalEstablished int64
	established      int64
	active           int64
	idle             int64
}

var version string
var cw ConnectionWatcher
var tx uint64

// https://stackoverflow.com/questions/51317122/how-to-get-number-of-idle-and-active-connections-in-go
// OnStateChange records open connections in response to connection
// state changes. Set net/http Server.ConnState to this method
// as value.
func (cw *ConnectionWatcher) OnStateChange(conn net.Conn, state http.ConnState) {
	switch state {
	case http.StateNew:
		metrics.Metrics.EstablishedConnections.Inc()
		metrics.Metrics.TotalConnEstablished.Inc()
	case http.StateClosed, http.StateHijacked:
		metrics.Metrics.EstablishedConnections.Dec()
	}
}

func beforeMisc(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		defer utils.PanicHandler(w)
		next(w, req)
	}
}

func beforeProxy(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		defer utils.PanicHandler(w)

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Max-Age", "1728000")
		w.Header().Set("Strict-Transport-Security", "max-age=86400")
		w.Header().Set("X-Powered-By", "ytproxy-go "+version+"-"+runtime.GOARCH)

		if req.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if req.Method != "GET" && req.Method != "HEAD" {
			w.WriteHeader(405)
			io.WriteString(w, "Only GET and HEAD requests are allowed.")
			return
		}

		metrics.Metrics.RequestCount.Inc()
		next(w, req)
	}
}

func init() {
	config.LoadConfig()
	log.StartLogger()
}

func main() {
	log.Logger.Trace().Any("config", config.Cfg).Msg("current config values")

	clientVerMsg := func(version string) string {
		return fmt.Sprintf("using %s Client for /videoplayback requests", version)
	}

	switch config.Cfg.Http_client_ver {
	case 2:
		log.Logger.Info().Msg(clientVerMsg("HTTP/2"))
		httpc.Client = httpc.H2client
	case 3:
		log.Logger.Info().Msg(clientVerMsg("HTTP/3"))
		httpc.Client = httpc.H3client
	default:
		log.Logger.Info().Msg(clientVerMsg("HTTP/1.1"))
		httpc.Client = httpc.H1_1client
	}

	mux := http.NewServeMux()

	// MISC ROUTES
	mux.HandleFunc("GET /", beforeMisc(routes.Root))
	mux.HandleFunc("GET /health", beforeMisc(routes.Health))
	mux.Handle("GET /metrics", routes.MetricsHandler())

	// PROXY ROUTES
	mux.HandleFunc("GET /videoplayback", beforeProxy(routes.Videoplayback))
	mux.HandleFunc("GET /vi/{id}/{name}", beforeProxy(routes.Vi))
	mux.HandleFunc("GET /vi_webp/{id}/{name}", beforeProxy(routes.Vi))
	mux.HandleFunc("GET /sb/{authority}/{id}/{storyboard}/{index}", beforeProxy(routes.Storyboards))
	mux.HandleFunc("GET /ggpht/", beforeProxy(routes.Ggpht))
	// The 3 Handles bellow use the same logic as `s_p` thumbnails.
	// Reference: https://github.com/iv-org/invidious/pull/5742
	//
	// Thumbnails used somewhere in Youtube, I have no idea where this is actually used.
	mux.HandleFunc("GET /s_p/{id}/{name}", beforeProxy(routes.S_p("i9")))
	// Thumbnails used in some Youtube playlists.
	mux.HandleFunc("GET /tvfilm_banner/{id}/{name}", beforeProxy(routes.S_p("i")))
	// Thumbnails used in some Youtube podcasts (a form of a Youtube playlist).
	mux.HandleFunc("GET /pl_c/{id}/{name}", beforeProxy(routes.S_p("i")))

	serverAddress := config.Cfg.Server.Host + ":" + config.Cfg.Server.Port
	srv := &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 1 * time.Hour,
		ConnState:    cw.OnStateChange,
		Addr:         serverAddress,
	}

	if config.Cfg.Server.Uds {
		uds_path := config.Cfg.Server.Uds_path

		syscall.Unlink(uds_path)
		socket_listener, err := net.Listen("unix", uds_path)
		if err != nil {
			log.Logger.Error().Err(err).Msg("failed to bind as a unix socket, please check the socket path")
		}

		defer socket_listener.Close()

		err = os.Chmod(uds_path, 0777)
		if err != nil {
			log.Logger.Error().Err(err).Msg("failed to set socket permissions to 777")
			return
		} else {
			log.Logger.Info().Msg("setting socket permissions to 777")
		}

		go func() {
			err := srv.Serve(socket_listener)
			if err != nil {
				log.Logger.Error().Err(err).Msg("failed to listen on unix domain socket")
			}
		}()

		// To allow everyone to access the socket
		log.Logger.Info().Str("unix_socket_path", uds_path).Msg("unix socket listening")
	}

	if config.Cfg.Server.Enable_http {
		port := config.Cfg.Server.Port
		host := config.Cfg.Server.Host

		log.Logger.Info().Str("port", port).Str("host", host).Msg("starting HTTP server")
		if err := srv.ListenAndServe(); err != nil {
			log.Logger.Fatal().Err(err).Msg("failed to start HTTP server")
		}
		log.Logger.Info().Str("port", port).Str("host", host).Msg("HTTP server started!")
	}
}
