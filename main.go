package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/keur/chillmailer/datastore"
	"github.com/keur/chillmailer/mailer"
	"github.com/keur/chillmailer/middleware"
	"github.com/keur/chillmailer/util"

	"github.com/go-chi/chi/v5"
	chiware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"
)

func setupLogger(ctx context.Context) (context.Context, *zerolog.Logger) {
	var output io.Writer
	if os.Getenv("ENV") == "local" {
		// Pretty logging when testing locally
		output = zerolog.ConsoleWriter{Out: os.Stdout}
	} else {
		output = os.Stdout
	}

	logger := zerolog.New(output).With().Timestamp().Logger()

	debug := os.Getenv("DEBUG")
	if util.StringIsNo(debug) {
		logger = log.Level(zerolog.InfoLevel)
	}
	return logger.WithContext(ctx), &logger
}

func setupRouter(ctx context.Context, logger *zerolog.Logger, ds datastore.Datastore) (context.Context, *chi.Mux) {
	r := chi.NewRouter()

	r.Use(chiware.RequestID)
	r.Use(chiware.RealIP)
	r.Use(chiware.Heartbeat("/healthz"))
	if logger != nil {
		r.Use(hlog.NewHandler(*logger))
		r.Use(hlog.UserAgentHandler("user_agent"))
		r.Use(hlog.RequestIDHandler("req_id", "Request-Id"))
	}

	r.Use(chiware.Logger)
	r.Use(chiware.Recoverer)

	// Public paths
	workDir, _ := os.Getwd()
	filesDir := http.Dir(filepath.Join(workDir, "static"))
	fileserver(r, "/static", filesDir)

	r.Post("/subscribe", serveSubscribe(ds))
	r.Get("/unsubscribe/{listName}/{email}/{unsubToken}", serveUnsubscribe(ds))

	r.Get("/", func(writer http.ResponseWriter, req *http.Request) {
		http.Redirect(writer, req, "/admin", http.StatusMovedPermanently)
	})

	// Shared rate limiter for outgoing emails. AWS SES limits us to 1 email per second
	limiter := rate.NewLimiter(1, 1)

	// Admin routes require basic auth
	adminRouter := r.With(middleware.BasicAuth)
	mailCanceller := mailer.NewMailCanceller()
	adminRouter.Route("/admin", func(r chi.Router) {
		r.Get("/", serveIndex(ds))
		r.Get("/list/display/{listName}", serveDisplayList(ds, mailCanceller))
		r.Get("/list/cancel/{listName}", serveCancelList(logger, mailCanceller))
		r.Post("/create-list", serveCreateList(ds))
		r.Post("/enqueue-mail", serveEnqueueMail(logger, ds, mailCanceller, limiter))
	})

	return ctx, r
}

func main() {
	var (
		serverCtx, logger = setupLogger(context.Background())
	)

	databaseFile := util.GetenvOr("DATABASE_FILE", "chillmailer.db")
	datastore, err := datastore.NewSqlite(databaseFile)
	if err != nil {
		logger.Panic().Err(err).Msg("sqlite database creation failed!")
	}
	defer datastore.Close()

	err = datastore.InitializeDatabase()
	if err != nil {
		logger.Panic().Err(err).Msg("could not initialize sqlite database file!")
	}
	logger.Info().Msgf("Initialized database file: %s", databaseFile)

	serverCtx, r := setupRouter(serverCtx, logger, datastore)
	serverCtx, cancel := context.WithCancel(serverCtx)
	defer cancel()

	port, valid := os.LookupEnv("PORT")
	if !valid {
		port = "7171"
	}

	debug := os.Getenv("DEBUG")
	addr := ""
	if util.StringIsYes(debug) {
		addr = "localhost"
	}
	srv := &http.Server{
		Addr:         addr + ":" + port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 20 * time.Second,
	}
	srv.BaseContext = func(_ net.Listener) context.Context {
		return serverCtx
	}
	logger.Info().Msg("Starting web server")
	err = srv.ListenAndServe()
	if err != nil {
		logger.Panic().Err(err).Msg("HTTP server start failed!")
	}
}

func fileserver(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit any URL parameters.")
	}

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", 301).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fs := http.StripPrefix(pathPrefix, http.FileServer(root))
		fs.ServeHTTP(w, r)
	})
}

type IndexData struct {
	Infos []datastore.MailingListInfo
}

func serveIndex(ds datastore.Datastore) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		infos, err := ds.QueryAllMailingLists()
		if err != nil {
			util.ServerError(w, err)
			return
		}
		tmpl, err := util.NewTemplate("index.html")
		if err != nil {
			util.ServerError(w, err)
			return
		}
		if err = tmpl.Execute(w, IndexData{Infos: infos}); err != nil {
			util.ServerError(w, err)
			return
		}
	})
}

type SubscribePayload struct {
	Email string `json:"email"`
	List  string `json:"list"`
}

func serveSubscribe(ds datastore.Datastore) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			util.ServerError(w, err)
			return
		}
		payload := &SubscribePayload{}
		err = json.Unmarshal(body, payload)
		if err != nil {
			util.ServerError(w, err)
			return
		}
		if !util.IsEmailValid(payload.Email) {
			util.UserError(w, fmt.Sprintf("Provided invalid email: %s", payload.Email))
			return
		}
		listID, err := ds.GetMailingListID(payload.List)
		if err != nil {
			util.ServerError(w, err)
			return
		}
		if listID == datastore.MailingListNoExist {
			util.UserError(w, fmt.Sprintf("Provided invalid mailing list: %s", payload.List))
			return
		}

		if err = ds.SubscribeToMailingList(listID, payload.Email); err != nil {
			util.ServerError(w, err)
			return
		}

		// CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		util.GoBackWhereYouCameFrom(w, r)
	})
}

func serveUnsubscribe(ds datastore.Datastore) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		listName := chi.URLParam(r, "listName")
		email := chi.URLParam(r, "email")
		unsubToken := chi.URLParam(r, "unsubToken")
		listID, err := ds.GetMailingListID(listName)
		if err != nil {
			util.ServerError(w, err)
			return
		}
		if listID == datastore.MailingListNoExist {
			util.UserError(w, fmt.Sprintf("Provided invalid mailing list: %s", listName))
			return
		}

		if err = ds.UnsubscribeRequest(listID, email, unsubToken); err != nil {
			if err == sql.ErrNoRows {
				util.NotFound(w, fmt.Sprintf("Email %s not found on list %s", email, listName))
				return
			} else if err == datastore.ErrorBadToken {
				util.Forbidden(w, "Bad token provided")
				return
			}
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		io.WriteString(w, "You have been unsubscribed")
		util.GoBackWhereYouCameFrom(w, r)
	})
}

type DisplayListInfo struct {
	ListName        string
	Subscribers     []datastore.SubscriberInfo
	HasPendingBlast bool
}

func serveDisplayList(ds datastore.Datastore, mc *mailer.MailCanceller) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		listName := chi.URLParam(r, "listName")
		listID, err := ds.GetMailingListID(listName)
		if err != nil {
			util.ServerError(w, err)
			return
		}
		if listID == datastore.MailingListNoExist {
			util.UserError(w, fmt.Sprintf("Provided mailing list %s invalid", listName))
			return
		}

		subs, err := ds.QueryMailingListSubscriberInfo(listID)
		if err != nil {
			util.ServerError(w, err)
			return
		}
		pageData := DisplayListInfo{
			ListName:        listName,
			Subscribers:     subs,
			HasPendingBlast: mc.ListHasContext(listName),
		}
		tmpl, err := util.NewTemplate("list.html")
		if err != nil {
			util.ServerError(w, err)
			return
		}
		if err = tmpl.Execute(w, &pageData); err != nil {
			util.ServerError(w, err)
			return
		}
	})
}

func serveCreateList(ds datastore.Datastore) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			util.ServerError(w, err)
			return
		}
		name := util.FormValue(r, "name")
		description := util.FormValue(r, "description")
		if name == "" || description == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, err = ds.CreateMailingList(name, description)
		if err != nil {
			util.ServerError(w, err)
			return
		}
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	})
}

func serveCancelList(logger *zerolog.Logger, mc *mailer.MailCanceller) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		listName := chi.URLParam(r, "listName")
		logger.Info().Msgf("Sending cancel for list %s", listName)
		mc.CancelMailingList(listName)
		redirectLink := filepath.Join("/admin/list/display/", listName)
		http.Redirect(w, r, redirectLink, http.StatusSeeOther)
	})
}

func serveEnqueueMail(logger *zerolog.Logger, ds datastore.Datastore, mc *mailer.MailCanceller, limiter *rate.Limiter) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			util.ServerError(w, err)
			return
		}
		listName := util.FormValue(r, "list_name")
		subject := util.FormValue(r, "subject")
		body := util.FormValue(r, "body")
		if listName == "" || subject == "" || body == "" {
			util.UserError(w, "Provided invalid form data")
			return
		}
		listID, err := ds.GetMailingListID(listName)
		if err != nil {
			util.ServerError(w, err)
			return
		}
		if listID == datastore.MailingListNoExist {
			util.UserError(w, fmt.Sprintf("Provided invalid mailing list: %s", listName))
			return
		}
		subscribers, err := ds.QueryMailingListSubscriberInfo(listID)
		if err != nil {
			util.ServerError(w, err)
			return
		}
		mxDomain, err := util.GetenvOrError("MX_DOMAIN")
		if err != nil {
			util.ServerError(w, err)
			return
		}
		webRoot := util.GetWebRoot(r)
		go func() {
			cancelCtx := mc.ContextForMailingList(listName)
			select {
			case <-time.After(30 * time.Second):
				rateCtx := context.Background()
				sender := fmt.Sprintf("chillmailer-%s@%s", listName, mxDomain)
				for _, sub := range subscribers {
					limiter.Wait(rateCtx)
					unsubscribeLink := webRoot + filepath.Join("/unsubscribe", listName, sub.Email, sub.UnsubToken)
					if err = mailer.SendMail(sender, sub.Email, subject, body, unsubscribeLink); err != nil {
						logger.Error().Err(err)
					} else {
						logger.Info().Msgf("Successfully sent email to %s", sub.Email)
					}
				}
			case <-cancelCtx.Done():
				logger.Info().Msgf("Emails to list %s have been cancelled", listName)
			}
		}()
		redirectLink := filepath.Join("/admin/list/display/", listName)
		http.Redirect(w, r, redirectLink, http.StatusSeeOther)
	})
}
