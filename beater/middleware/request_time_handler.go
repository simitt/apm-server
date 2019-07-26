package middleware

import (
	"time"

	"github.com/elastic/apm-server/beater/request"
	"github.com/elastic/apm-server/utility"
)

func RequestTimeHandler() Middleware {
	return func(h request.Handler) request.Handler {
		return func(c *request.Context) {
			c.Req = c.Req.WithContext(utility.ContextWithRequestTime(c.Req.Context(), time.Now()))
			h(c)
		}
	}
}
