package middleware

import (
	"crypto/subtle"
	"net/http"
	"os"

	"github.com/keur/chillmailer/util"

	"github.com/rs/zerolog/log"
)

// BasicAuth implements a simple middleware handler for adding basic http auth to a route.
func BasicAuth(next http.Handler) http.Handler {
	var adminUser = os.Getenv("ADMIN_USER")
	var adminPass = os.Getenv("ADMIN_PASS")
	if adminUser == "" {
		adminUser = "admin"
	}
	if adminPass == "" {
		adminPass = "password"
		debug := os.Getenv("DEBUG")
		if util.StringIsNo(debug) {
			log.Panic().Msg("Server running in production with default password!")
		}
	}
	authFailed := func(w http.ResponseWriter) {
		w.Header().Add("WWW-Authenticate", `Basic realm="Restricted"`)
		w.WriteHeader(http.StatusUnauthorized)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok {
			authFailed(w)
			return
		}

		if user != adminUser || subtle.ConstantTimeCompare([]byte(pass), []byte(adminPass)) != 1 {
			authFailed(w)
			return
		}

		next.ServeHTTP(w, r)
	})
}
