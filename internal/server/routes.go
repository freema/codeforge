package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// sessionRoutes is the part of *handlers.SessionHandler served under /sessions.
type sessionRoutes interface {
	List(http.ResponseWriter, *http.Request)
	Create(http.ResponseWriter, *http.Request)
	Get(http.ResponseWriter, *http.Request)
	Instruct(http.ResponseWriter, *http.Request)
	Cancel(http.ResponseWriter, *http.Request)
	Review(http.ResponseWriter, *http.Request)
	PostReviewComments(http.ResponseWriter, *http.Request)
	CreatePR(http.ResponseWriter, *http.Request)
	PushToPR(http.ResponseWriter, *http.Request)
	GetPRStatus(http.ResponseWriter, *http.Request)
	Diff(http.ResponseWriter, *http.Request)
}

// mountSessionRoutes registers the /sessions routes. The ownership middleware is
// attached inside the /{sessionID} subrouter: chi matches URL params while
// routing, so middleware registered with r.Use above the pattern would see an
// empty {sessionID} and let every tenant through.
func mountSessionRoutes(r chi.Router, h sessionRoutes, ownership, rateLimitMw func(http.Handler) http.Handler) {
	r.Route("/sessions", func(r chi.Router) {
		r.Get("/", h.List)
		if rateLimitMw != nil {
			r.With(rateLimitMw).Post("/", h.Create)
		} else {
			r.Post("/", h.Create)
		}

		r.Route("/{sessionID}", func(r chi.Router) {
			r.Use(ownership) // tenant may touch only its own sessions
			r.Get("/", h.Get)
			r.Post("/instruct", h.Instruct)
			r.Post("/cancel", h.Cancel)
			r.Post("/review", h.Review)
			r.Post("/post-review", h.PostReviewComments)
			r.Post("/create-pr", h.CreatePR)
			r.Post("/push", h.PushToPR)
			r.Get("/pr-status", h.GetPRStatus)
			r.Get("/diff", h.Diff)
		})
	})
}
