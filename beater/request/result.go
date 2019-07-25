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

package request

import (
	"net/http"

	"github.com/elastic/beats/libbeat/monitoring"

	"github.com/pkg/errors"
)

var (
	serverMetrics = monitoring.Default.NewRegistry("apm-server.server", monitoring.PublishExpvar)
	counter       = func(s string) *monitoring.Int {
		return monitoring.NewInt(serverMetrics, s)
	}

	RequestCounter = counter("request.count")

	ResponseCounter = counter("response.count")

	ResponseErrors    = counter("response.errors.count")
	ResponseSuccesses = counter("response.valid.count")

	ResponseOk       = counter("response.valid.ok")
	ResponseAccepted = counter("response.valid.accepted")

	InternalErrorCounter      = counter("response.errors.internal")
	ForbiddenCounter          = counter("response.errors.forbidden")
	RequestTooLargeCounter    = counter("response.errors.toolarge")
	DecodeCounter             = counter("response.errors.decode")
	ValidateCounter           = counter("response.errors.validate")
	RateLimitCounter          = counter("response.errors.ratelimit")
	MethodNotAllowedCounter   = counter("response.errors.method")
	FullQueueCounter          = counter("response.errors.queue")
	ServerShuttingDownCounter = counter("response.errors.closed")
	UnauthorizedCounter       = counter("response.errors.unauthorized")

	OkResult = Result{
		Code:    http.StatusOK,
		Counter: ResponseOk,
	}
	AcceptedResult = Result{
		Code:    http.StatusAccepted,
		Counter: ResponseAccepted,
	}
	InternalErrorResult = func(err error) Result {
		return Result{
			Err:     errors.Wrap(err, "internal error"),
			Code:    http.StatusInternalServerError,
			Counter: InternalErrorCounter,
		}
	}
	ForbiddenResult = func(err error) Result {
		return Result{
			Err:     errors.Wrap(err, "forbidden request"),
			Code:    http.StatusForbidden,
			Counter: ForbiddenCounter,
		}
	}
	UnauthorizedResult = Result{
		Err:     errors.New("invalid token"),
		Code:    http.StatusUnauthorized,
		Counter: UnauthorizedCounter,
	}
	RequestTooLargeResult = Result{
		Err:     errors.New("request body too large"),
		Code:    http.StatusRequestEntityTooLarge,
		Counter: RequestTooLargeCounter,
	}
	CannotDecodeResult = func(err error) Result {
		return Result{
			Err:     errors.Wrap(err, "data decoding error"),
			Code:    http.StatusBadRequest,
			Counter: DecodeCounter,
		}
	}
	CannotValidateResult = func(err error) Result {
		return Result{
			Err:     errors.Wrap(err, "data validation error"),
			Code:    http.StatusBadRequest,
			Counter: ValidateCounter,
		}
	}
	RateLimitedResult = Result{
		Err:     errors.New("too many requests"),
		Code:    http.StatusTooManyRequests,
		Counter: RateLimitCounter,
	}
	MethodNotAllowedResult = Result{
		Err:     errors.New("only POST requests are supported"),
		Code:    http.StatusMethodNotAllowed,
		Counter: MethodNotAllowedCounter,
	}
	FullQueueResult = func(err error) Result {
		return Result{
			Err:     errors.Wrap(err, "queue is full"),
			Code:    http.StatusServiceUnavailable,
			Counter: FullQueueCounter,
		}
	}
	ServerShuttingDownResult = func(err error) Result {
		return Result{
			Err:     errors.New("server is shutting down"),
			Code:    http.StatusServiceUnavailable,
			Counter: ServerShuttingDownCounter,
		}
	}
)

type Result struct {
	Code    int
	Counter *monitoring.Int
	Err     error
	Body    interface{}
}

func (r Result) WriteTo(c *Context) {
	if r.Code >= http.StatusBadRequest || r.Err != nil {
		//TODO: remove extra handling when changing logs
		err := map[string]string{"error": r.Err.Error()}
		c.WriteWithError(r.Err.Error(), err, r.Code)
		return
	}
	c.Write(r.Body, r.Code)
}
