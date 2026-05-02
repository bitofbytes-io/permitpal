package server

import (
	"log/slog"
	"net/http"

	"github.com/drywaters/permitpal/internal/auth"
	"github.com/drywaters/permitpal/internal/config"
	"github.com/drywaters/permitpal/internal/handler"
	"github.com/drywaters/permitpal/internal/middleware"
	"github.com/drywaters/permitpal/internal/repository"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	cfg    *config.Config
	store  repository.Store
	logger *slog.Logger
}

func New(cfg *config.Config, store repository.Store, logger *slog.Logger) *Server {
	return &Server{cfg: cfg, store: store, logger: logger}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(middleware.RequireSameOrigin)
	r.Use(middleware.LimitBodyBytes(16 * 1024))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	fileServer := http.FileServer(http.Dir("static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fileServer))

	authManager := auth.NewManager(s.cfg)
	authHandler := handler.NewAuthHandler(authManager)
	r.Get("/login", authHandler.LoginPage)
	r.Post("/login", authHandler.Login)
	r.Post("/logout", authHandler.Logout)

	dashboardHandler := handler.NewDashboardHandler(s.store)
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(authManager))
		r.Get("/", dashboardHandler.Dashboard)
		r.Post("/profile", dashboardHandler.UpdateProfile)
		r.Post("/requirements/{key}", dashboardHandler.UpdateRequirement)
	})

	return r
}
