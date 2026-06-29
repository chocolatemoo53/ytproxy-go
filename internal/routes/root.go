package routes

import (
	"io"
	"net/http"
)

func Root(w http.ResponseWriter, req *http.Request) {
	const msg = `
	HTTP youtube proxy for https://inv.nadeko.net
	https://git.nadeko.net/Fijxu/ytproxy-go

	Routes:
	/stats
	/health`
	_, _ = io.WriteString(w, msg)
}
