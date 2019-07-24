// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package beater

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/ryanuber/go-glob"

	"github.com/elastic/beats/libbeat/monitoring"

	"github.com/elastic/apm-server/beater/headers"
	"github.com/elastic/apm-server/beater/request"
	"github.com/elastic/apm-server/utility"
)

var (
	supportedHeaders = fmt.Sprintf("%s, %s, %s", headers.ContentType, headers.ContentEncoding, headers.Accept)
	supportedMethods = fmt.Sprintf("%s, %s", http.MethodPost, http.MethodOptions)
)

var (
	serverMetrics = monitoring.Default.NewRegistry("apm-server.server", monitoring.PublishExpvar)
	counter       = func(s string) *monitoring.Int {
		return monitoring.NewInt(serverMetrics, s)
	}
	requestCounter            = counter("request.count")
	responseCounter           = counter("response.count")
	responseErrors            = counter("response.errors.count")
	responseSuccesses         = counter("response.valid.count")
	responseOk                = counter("response.valid.ok")
	responseAccepted          = counter("response.valid.accepted")
	internalErrorCounter      = counter("response.errors.internal")
	forbiddenCounter          = counter("response.errors.forbidden")
	requestTooLargeCounter    = counter("response.errors.toolarge")
	decodeCounter             = counter("response.errors.decode")
	validateCounter           = counter("response.errors.validate")
	rateLimitCounter          = counter("response.errors.ratelimit")
	methodNotAllowedCounter   = counter("response.errors.method")
	fullQueueCounter          = counter("response.errors.queue")
	serverShuttingDownCounter = counter("response.errors.closed")
	unauthorizedCounter       = counter("response.errors.unauthorized")

	okResponse = serverResponse{
		code:    http.StatusOK,
		counter: responseOk,
	}
	acceptedResponse = serverResponse{
		code:    http.StatusAccepted,
		counter: responseAccepted,
	}
	internalErrorResponse = func(err error) serverResponse {
		return serverResponse{
			err:     errors.Wrap(err, "internal error"),
			code:    http.StatusInternalServerError,
			counter: internalErrorCounter,
		}
	}
	forbiddenResponse = func(err error) serverResponse {
		return serverResponse{
			err:     errors.Wrap(err, "forbidden request"),
			code:    http.StatusForbidden,
			counter: forbiddenCounter,
		}
	}
	unauthorizedResponse = serverResponse{
		err:     errors.New("invalid token"),
		code:    http.StatusUnauthorized,
		counter: unauthorizedCounter,
	}
	requestTooLargeResponse = serverResponse{
		err:     errors.New("request body too large"),
		code:    http.StatusRequestEntityTooLarge,
		counter: requestTooLargeCounter,
	}
	cannotDecodeResponse = func(err error) serverResponse {
		return serverResponse{
			err:     errors.Wrap(err, "data decoding error"),
			code:    http.StatusBadRequest,
			counter: decodeCounter,
		}
	}
	cannotValidateResponse = func(err error) serverResponse {
		return serverResponse{
			err:     errors.Wrap(err, "data validation error"),
			code:    http.StatusBadRequest,
			counter: validateCounter,
		}
	}
	rateLimitedResponse = serverResponse{
		err:     errors.New("too many requests"),
		code:    http.StatusTooManyRequests,
		counter: rateLimitCounter,
	}
	methodNotAllowedResponse = serverResponse{
		err:     errors.New("only POST requests are supported"),
		code:    http.StatusMethodNotAllowed,
		counter: methodNotAllowedCounter,
	}
	fullQueueResponse = func(err error) serverResponse {
		return serverResponse{
			err:     errors.Wrap(err, "queue is full"),
			code:    http.StatusServiceUnavailable,
			counter: fullQueueCounter,
		}
	}
	serverShuttingDownResponse = func(err error) serverResponse {
		return serverResponse{
			err:     errors.New("server is shutting down"),
			code:    http.StatusServiceUnavailable,
			counter: serverShuttingDownCounter,
		}
	}
)

type serverResponse struct {
	code    int
	counter *monitoring.Int
	err     error
	body    interface{}
}

func (r serverResponse) writeTo(c *request.Context) {
	if r.code >= http.StatusBadRequest || r.err != nil {
		//TODO: remove extra handling when changing logs
		err := map[string]string{"error": r.err.Error()}
		c.WriteWithError(r.err.Error(), err, r.code)
		return
	}
	c.Write(r.body, r.code)
}

func requestTimeHandler(h Handler) Handler {
	return func(c *request.Context) {
		c.Req = c.Req.WithContext(utility.ContextWithRequestTime(c.Req.Context(), time.Now()))
		h(c)
	}
}

func killSwitchHandler(killSwitch bool, h Handler) Handler {
	return func(c *request.Context) {
		if killSwitch {
			h(c)
		} else {
			forbiddenResponse(errors.New("endpoint is disabled")).writeTo(c)
		}
	}
}

func authHandler(secretToken string, h Handler) Handler {
	return func(c *request.Context) {
		if !isAuthorized(c.Req, secretToken) {
			unauthorizedResponse.writeTo(c)
			return
		}
		h(c)
	}
}

// isAuthorized checks the Authorization header. It must be in the form of:
//   Authorization: Bearer <secret-token>
// Bearer must be part of it.
func isAuthorized(req *http.Request, secretToken string) bool {
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

func corsHandler(allowedOrigins []string, h Handler) Handler {

	var isAllowed = func(origin string) bool {
		for _, allowed := range allowedOrigins {
			if glob.Glob(allowed, origin) {
				return true
			}
		}
		return false
	}

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

			okResponse.writeTo(c)

		} else if validOrigin {
			// we need to check the origin and set the ACAO header in both the OPTIONS preflight and the actual request
			c.Header().Set(headers.AccessControlAllowOrigin, origin)
			h(c)

		} else {
			forbiddenResponse(errors.New("origin: '" + origin + "' is not allowed")).writeTo(c)
		}
	}
}
