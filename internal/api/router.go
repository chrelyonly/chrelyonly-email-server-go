package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// SetupRouter 初始化内部 REST API 路由
func SetupRouter(handler *APIHandler) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/internal/api/v1", func(r chi.Router) {
		r.Get("/health", handler.HealthCheck)

		r.Route("/mail", func(r chi.Router) {
			r.Post("/send", handler.SendMail)
			r.Get("/", handler.ListMails)
			r.Get("/{messageId}", handler.GetMailDetail)
			r.Delete("/{messageId}", handler.DeleteMail)
			r.Get("/attachments/{id}", handler.DownloadAttachment)
		})

		r.Route("/accounts", func(r chi.Router) {
			r.Get("/", handler.ListAccounts)
			r.Post("/", handler.CreateAccount)
			r.Delete("/{id}", handler.DeleteAccount)
		})

		r.Route("/domains", func(r chi.Router) {
			r.Get("/", handler.ListDomains)
			r.Post("/", handler.CreateDomain)
			r.Delete("/{id}", handler.DeleteDomain)
		})
	})

	return r
}
