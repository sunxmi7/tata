package tata

import (
	_ "embed"
	"net/http"
)

//go:embed admin.html
var adminHTML []byte

//go:embed demo.html
var demoHTML []byte

func (a *App) adminPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "http://127.0.0.1:5666/", http.StatusTemporaryRedirect)
}

func (a *App) demoPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(demoHTML)
}
