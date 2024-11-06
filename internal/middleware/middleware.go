package middleware

import (
	"net/http"
)

// Middleware interface defines how middleware components should behave
type Middleware interface {
	Wrap(http.Handler) http.Handler
}

// Chain creates a new handler by chaining multiple middleware together
func Chain(h http.Handler, middleware ...Middleware) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i].Wrap(h)
	}
	return h
}
