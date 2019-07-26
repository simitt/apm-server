package middleware

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/ryanuber/go-glob"

	"github.com/elastic/apm-server/beater/headers"
	"github.com/elastic/apm-server/beater/request"
)

var (
	supportedHeaders = fmt.Sprintf("%s, %s, %s", headers.ContentType, headers.ContentEncoding, headers.Accept)
	supportedMethods = fmt.Sprintf("%s, %s", http.MethodPost, http.MethodOptions)
)

func CorsHandler(allowedOrigins []string) Middleware {

	var isAllowed = func(origin string) bool {
		for _, allowed := range allowedOrigins {
			if glob.Glob(allowed, origin) {
				return true
			}
		}
		return false
	}

	return func(h request.Handler) request.Handler {
		return func(c *request.Context) {

			// origin header is always set by the browser
			origin := c.Req.Header.Get(headers.Origin)
			validOrigin := isAllowed(origin)

			if c.Req.Method == http.MethodOptions {

				// setting the ACAO header is the way to tell the browser to go ahead with the request
				if validOrigin {
					// do not set the configured origin(s), echo the received origin instead
					c.Header().Set(headers.AccessControlAllowOrigin, origin)
				}

				// tell browsers to cache response requestHeaders for up to 1 hour (browsers might ignore this)
				c.Header().Set(headers.AccessControlMaxAge, "3600")
				// origin must be part of the cache key so that we can handle multiple allowed origins
				c.Header().Set(headers.Vary, "Origin")

				// required if Access-Control-Request-Method and Access-Control-Request-Headers are in the requestHeaders
				c.Header().Set(headers.AccessControlAllowMethods, supportedMethods)
				c.Header().Set(headers.AccessControlAllowHeaders, supportedHeaders)

				c.Header().Set(headers.ContentLength, "0")

				request.OkResult.WriteTo(c)

			} else if validOrigin {
				// we need to check the origin and set the ACAO header in both the OPTIONS preflight and the actual request
				c.Header().Set(headers.AccessControlAllowOrigin, origin)
				h(c)

			} else {
				request.ForbiddenResult(errors.New("origin: '" + origin + "' is not allowed")).WriteTo(c)
			}
		}
	}
}
