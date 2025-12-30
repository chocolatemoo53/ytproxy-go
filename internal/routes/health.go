package routes

import (
	"io"
	"net/http"
)

func Health(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(200)
	_, _ = io.WriteString(w, "OK")
}
