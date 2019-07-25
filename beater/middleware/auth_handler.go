package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/elastic/apm-server/beater/headers"
	"github.com/elastic/apm-server/beater/request"
)

func AuthHandler(secretToken string, h request.Handler) request.Handler {
	return func(c *request.Context) {
		if !IsAuthorized(c.Req, secretToken) {
			request.UnauthorizedResult.WriteTo(c)
			return
		}
		h(c)
	}
}

// IsAuthorized checks the Authorization header. It must be in the form of:
//   Authorization: Bearer <secret-token>
// Bearer must be part of it.
func IsAuthorized(req *http.Request, secretToken string) bool {
	// No token configured
	if secretToken == "" {
		return true
	}
	header := req.Header.Get(headers.Authorization)
	parts := strings.Split(header, " ")
	if len(parts) != 2 || parts[0] != headers.Bearer {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(secretToken)) == 1
}
