package routes

import (
	"net/http"
)

func Options(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
}
