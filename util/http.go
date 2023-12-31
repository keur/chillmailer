package util

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/rs/zerolog/log"
)

func GetWebRoot(r *http.Request) string {
	var proto string = "http"
	if xproto := r.Header.Get("X-Forwarded-Proto"); xproto != "" {
		proto = xproto
	} else if r.TLS != nil {
		proto = "https"
	}
	return proto + "://" + r.Host
}

func NewTemplate(filename string) (*template.Template, error) {
	dir, _ := os.Getwd()
	templateFile := filepath.Join(dir, "template", filename)
	return template.ParseFiles(templateFile)
}

func FormValue(r *http.Request, name string) string {
	return strings.TrimSpace(r.FormValue(name))
}

func ServerError(w http.ResponseWriter, err error) {
	log.Error().Err(err).Msg("Server Error")
	requestError(w, http.StatusInternalServerError, err.Error())
}

func UserError(w http.ResponseWriter, msg string) {
	requestError(w, http.StatusBadRequest, msg)
}

func NotFound(w http.ResponseWriter, msg string) {
	requestError(w, http.StatusNotFound, msg)
}

func Forbidden(w http.ResponseWriter, msg string) {
	requestError(w, http.StatusForbidden, msg)
}

func GoBackWhereYouCameFrom(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, r.Header.Get("Referer"), http.StatusFound)
}

func requestError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	io.WriteString(w, msg)
}
