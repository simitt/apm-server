package middleware

import "github.com/elastic/apm-server/beater/request"

type Middleware func(request.Handler) request.Handler

func WithMiddleware(h request.Handler, m ...Middleware) request.Handler {
	for i := len(m) - 1; i >= 0; i-- {
		h = m[i](h)
	}
	return h
}
