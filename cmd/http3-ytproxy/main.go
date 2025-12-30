package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"syscall"
	"time"

	"git.nadeko.net/Fijxu/http3-ytproxy/internal/config"
	"git.nadeko.net/Fijxu/http3-ytproxy/internal/httpc"
	"git.nadeko.net/Fijxu/http3-ytproxy/internal/metrics"
	"git.nadeko.net/Fijxu/http3-ytproxy/internal/paths"
	"git.nadeko.net/Fijxu/http3-ytproxy/internal/utils"
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
		w.Header().Set("X-Powered-By", "http3-ytproxy "+version+"-"+runtime.GOARCH)

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
}

func main() {
	log.Printf("[INFO] Current config values: %+v\n", config.Cfg)

	switch config.Cfg.Http_client_ver {
	case 1:
		log.Println("[INFO] Using HTTP/1.1 Client")
		httpc.Client = httpc.H1_1client
	case 2:
		log.Println("[INFO] Using HTTP/2 Client")
		httpc.Client = httpc.H2client
	case 3:
		log.Println("[INFO] Using HTTP/3 Client")
		httpc.Client = httpc.H3client
	default:
		log.Println("[INFO] Using HTTP/1.1 Client")
		httpc.Client = httpc.H1_1client
	}

	mux := http.NewServeMux()

	// MISC ROUTES
	mux.HandleFunc("/", beforeMisc(paths.Root))
	mux.HandleFunc("/health", beforeMisc(paths.Health))

	metrics.Register()

	mux.Handle("/metrics", paths.MetricsHandler())

	// PROXY ROUTES
	mux.HandleFunc("/videoplayback", beforeProxy(paths.Videoplayback))
	mux.HandleFunc("/vi/", beforeProxy(paths.Vi))
	mux.HandleFunc("/vi_webp/", beforeProxy(paths.Vi))
	mux.HandleFunc("/sb/", beforeProxy(paths.Vi))
	mux.HandleFunc("/ggpht/", beforeProxy(paths.Ggpht))
	mux.HandleFunc("/a/", beforeProxy(paths.Ggpht))
	mux.HandleFunc("/ytc/", beforeProxy(paths.Ggpht))

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
			log.Println("[ERROR] Failed to bind to UDS, please check the socket path", err.Error())
		}

		defer socket_listener.Close()

		err = os.Chmod(uds_path, 0777)
		if err != nil {
			log.Println("[ERROR] Failed to set socket permissions to 777:", err.Error())
			return
		} else {
			log.Println("[INFO] Setting socket permissions to 777")
		}

		go func() {
			err := srv.Serve(socket_listener)
			if err != nil {
				log.Println("[ERROR] Failed to listen serve UDS:", err)
			}
		}()

		// To allow everyone to access the socket
		log.Println("[INFO] Unix socket listening at:", uds_path)
	}

	if config.Cfg.Server.Enable_http {
		log.Println("[INFO] Serving HTTP server at port", config.Cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil {
			log.Fatalf("[FATAL] Failed to listen on '%s:%s': %s\n", config.Cfg.Server.Host, config.Cfg.Server.Port, err)
		}
	}
}
