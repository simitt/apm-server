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

package intake

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/time/rate"

	"github.com/elastic/beats/libbeat/monitoring"

	"github.com/elastic/apm-server/beater/headers"
	"github.com/elastic/apm-server/beater/request"
	"github.com/elastic/apm-server/decoder"
	"github.com/elastic/apm-server/processor/stream"
	"github.com/elastic/apm-server/publish"
	"github.com/elastic/apm-server/utility"
)

func NewHandler(dec decoder.ReqDecoder, processor *stream.Processor, rlc *RateLimitCache, report publish.Reporter) request.Handler {
	return func(c *request.Context) {

		serr := validateRequest(c.Req)
		if serr != nil {
			sendError(c, serr)
			return
		}

		rl, serr := rateLimit(c.Req, rlc)
		if serr != nil {
			sendError(c, serr)
			return
		}

		reader, serr := bodyReader(c.Req)
		if serr != nil {
			sendError(c, serr)
			return
		}

		// extract metadata information from the request, like user-agent or remote address
		reqMeta, err := dec(c.Req)
		if err != nil {
			sr := stream.Result{}
			sr.Add(err)
			sendResponse(c, &sr)
			return
		}
		res := processor.HandleStream(c.Req.Context(), rl, reqMeta, reader, report)

		sendResponse(c, res)
	}
}

func extractResult(sr *stream.Result) (int, *monitoring.Int) {
	var code = http.StatusAccepted
	var monInt = request.ResponseAccepted
	set := func(c int, i *monitoring.Int) {
		if c > code {
			code = c
			monInt = i
		}
	}

	for _, err := range sr.Errors {
		switch err.Type {
		case stream.MethodForbiddenErrType:
			set(http.StatusBadRequest, request.MethodNotAllowedCounter)
		case stream.InputTooLargeErrType:
			set(http.StatusBadRequest, request.RequestTooLargeCounter)
		case stream.InvalidInputErrType:
			set(http.StatusBadRequest, request.ValidateCounter)
		case stream.RateLimitErrType:
			set(http.StatusTooManyRequests, request.RateLimitCounter)
		case stream.QueueFullErrType:
			return http.StatusServiceUnavailable, request.FullQueueCounter
		case stream.ShuttingDownErrType:
			return http.StatusServiceUnavailable, request.ServerShuttingDownCounter
		default:
			set(http.StatusInternalServerError, request.InternalErrorCounter)
		}
	}
	return code, monInt
}

func sendResponse(c *request.Context, sr *stream.Result) {
	statusCode, monitoringInt := extractResult(sr)
	c.AddMonitoringCt(monitoringInt)

	if statusCode >= http.StatusBadRequest {
		// this signals to the client that we're closing the connection
		// but also signals to http.Server that it should close it:
		// https://golang.org/src/net/http/server.go#L1254
		c.Header().Add(headers.Connection, "Close")
		c.WriteWithError(sr, sr.Error(), statusCode)
		return
	}

	if _, ok := c.Req.URL.Query()["verbose"]; ok {
		c.Write(sr, statusCode)
		return
	}

	c.Write(nil, statusCode)
}

func sendError(c *request.Context, err *stream.Error) {
	sr := stream.Result{}
	sr.Add(err)
	sendResponse(c, &sr)
}

func validateRequest(r *http.Request) *stream.Error {
	if r.Method != http.MethodPost {
		return &stream.Error{
			Type:    stream.MethodForbiddenErrType,
			Message: "only POST requests are supported",
		}
	}

	if !strings.Contains(r.Header.Get(headers.ContentType), "application/x-ndjson") {
		return &stream.Error{
			Type:    stream.InvalidInputErrType,
			Message: fmt.Sprintf("invalid content type: '%s'", r.Header.Get(headers.ContentType)),
		}
	}
	return nil
}

func rateLimit(r *http.Request, rlc *RateLimitCache) (*rate.Limiter, *stream.Error) {
	if rl, ok := rlc.GetRateLimiter(utility.RemoteAddr(r)); ok {
		if !rl.Allow() {
			return nil, &stream.Error{
				Type:    stream.RateLimitErrType,
				Message: "rate limit exceeded",
			}
		}
		return rl, nil
	}
	return nil, nil
}

func bodyReader(r *http.Request) (io.ReadCloser, *stream.Error) {
	reader, err := decoder.CompressedRequestReader(r)
	if err != nil {
		return nil, &stream.Error{
			Type:    stream.InvalidInputErrType,
			Message: err.Error(),
		}
	}
	return reader, nil
}
