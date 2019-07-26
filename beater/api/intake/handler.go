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

	"github.com/pkg/errors"

	"golang.org/x/time/rate"

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

func extractResult(c *request.Context, sr *stream.Result, result *request.Result) {
	var code = http.StatusAccepted
	var name = request.NameResponseValidAccepted
	var keyword = request.KeywordResponseValidAccepted

	set := func(c int, n string, k string) {
		if c > code {
			code = c
			name = n
			keyword = k
		}
	}

	for _, err := range sr.Errors {
		switch err.Type {
		case stream.MethodForbiddenErrType:
			set(http.StatusBadRequest, request.NameResponseErrorsMethodNotAllowed, request.KeywordResponseErrorsMethodNotAllowed)
		case stream.InputTooLargeErrType:
			set(http.StatusBadRequest, request.NameResponseErrorsRequestTooLarge, request.KeywordResponseErrorsRequestTooLarge)
		case stream.InvalidInputErrType:
			set(http.StatusBadRequest, request.NameResponseErrorsValidate, request.KeywordResponseErrorsValidate)
		case stream.RateLimitErrType:
			set(http.StatusTooManyRequests, request.NameResponseErrorsRateLimit, request.KeywordResponseErrorsRateLimit)
		case stream.QueueFullErrType:
			set(http.StatusServiceUnavailable, request.NameResponseErrorsFullQueue, request.KeywordResponseErrorsFullQueue)
			break
		case stream.ShuttingDownErrType:
			set(http.StatusServiceUnavailable, request.NameResponseErrorsShuttingDown, request.KeywordResponseErrorsShuttingDown)
			break
		default:
			set(http.StatusInternalServerError, request.NameResponseErrorsInternal, request.KeywordResponseErrorsInternal)
		}
	}
	result.Set(name, code, keyword, nil, errors.New(sr.Error()))
}

func sendResponse(c *request.Context, sr *stream.Result) {
	var result request.Result
	extractResult(c, sr, &result)
	if result.Code >= http.StatusBadRequest {
		// this signals to the client that we're closing the connection
		// but also signals to http.Server that it should close it:
		// https://golang.org/src/net/http/server.go#L1254
		c.Header().Add(headers.Connection, "Close")
		result.Body = sr
	} else if _, ok := c.Req.URL.Query()["verbose"]; ok {
		result.Body = sr
	}
	c.Write(&result)
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
