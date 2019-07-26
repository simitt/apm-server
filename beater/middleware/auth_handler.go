package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/elastic/apm-server/beater/headers"
	"github.com/elastic/apm-server/beater/request"
)

func RequireAuthorization(token string) Middleware {
	return func(h request.Handler) request.Handler {
		return func(c *request.Context) {
			if !isAuthorized(c.Req, token) {
				request.UnauthorizedResult.WriteTo(c)
				return
			}
			h(c)
		}
	}
}

func SetAuthorization(token string) Middleware {
	return func(handler request.Handler) request.Handler {
		return func(c *request.Context) {
			c.Authorized = isAuthorized(c.Req, token)
			c.TokenSet = tokenSet(token)
		}

	}
}

// isAuthorized checks the Authorization header. It must be in the form of:
//   Authorization: Bearer <secret-token>
// Bearer must be part of it.
func isAuthorized(req *http.Request, token string) bool {
	// No token configured
	if tokenSet(token) {
		return true
	}
	header := req.Header.Get(headers.Authorization)
	parts := strings.Split(header, " ")
	if len(parts) != 2 || parts[0] != headers.Bearer {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(token)) == 1
}

func tokenSet(token string) bool {
	return token != ""
}
