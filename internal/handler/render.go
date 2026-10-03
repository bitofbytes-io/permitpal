package handler

import (
	"bytes"
	"net/http"

	"github.com/a-h/templ"
	"github.com/drywaters/permitpal/internal/middleware"
)

func render(w http.ResponseWriter, r *http.Request, component templ.Component) {
	renderStatus(w, r, http.StatusOK, component)
}

func renderStatus(w http.ResponseWriter, r *http.Request, status int, component templ.Component) {
	var buf bytes.Buffer
	if err := component.Render(r.Context(), &buf); err != nil {
		middleware.ServerError(w, r, "render failed", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}
