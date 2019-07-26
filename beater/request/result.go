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

	"github.com/pkg/errors"
)

const (
	NameRequestCount        = "request.count"
	NameResponseCount       = "response.count"
	NameResponseErrorsCount = "response.errors.count"
	NameResponseValidCount  = "response.valid.count"

	NameResponseValidOK          = "response.valid.ok"
	NameResponseValidAccepted    = "response.valid.accepted"
	NameResponseValidNotModified = "response.valid.notmodified"

	NameResponseErrorsForbidden          = "response.errors.forbidden"
	NameResponseErrorsUnauthorized       = "response.errors.unauthorized"
	NameResponseErrorsNotFound           = "response.errors.notfound"
	NameResponseErrorsInvalidQuery       = "response.errors.invalidquery"
	NameResponseErrorsRequestTooLarge    = "response.errors.toolarge"
	NameResponseErrorsDecode             = "response.errors.decode"
	NameResponseErrorsValidate           = "response.errors.validate"
	NameResponseErrorsRateLimit          = "response.errors.ratelimit"
	NameResponseErrorsMethodNotAllowed   = "response.errors.method"
	NameResponseErrorsFullQueue          = "response.errors.queue"
	NameResponseErrorsShuttingDown       = "response.errors.closed"
	NameResponseErrorsServiceUnavailable = "response.errors.unavailable"
	NameResponseErrorsInternal           = "response.errors.internal"

	// Keywords
	KeywordResponseValidOK       = "request ok"
	KeywordResponseValidAccepted = "request accepted"
	KeywordResponseNotModified   = "not modified"

	KeywordResponseErrorsForbidden          = "forbidden request"
	KeywordResponseErrorsUnauthorized       = "invalid token"
	KeywordResponseErrorsNotFound           = "404 page not found"
	KeywordResponseErrorsInvalidQuery       = "invalid query"
	KeywordResponseErrorsRequestTooLarge    = "request body too large"
	KeywordResponseErrorsDecode             = "data decoding error"
	KeywordResponseErrorsValidate           = "data validation error"
	KeywordResponseErrorsRateLimit          = "too many requests"
	KeywordResponseErrorsMethodNotAllowed   = "only POST requests are supported"
	KeywordResponseErrorsFullQueue          = "queue is full"
	KeywordResponseErrorsShuttingDown       = "server is shutting down"
	KeywordResponseErrorsServiceUnavailable = "service unavailable"
	KeywordResponseErrorsInternal           = "internal error"
)

func ResultFor(name string, result *Result) {
	ResultWithError(name, nil, result)
}

func ResultWithError(name string, err error, r *Result) {
	if r == nil {
		return
	}
	var body interface{}
	switch name {
	case NameResponseValidOK:
		r.Set(NameResponseValidOK, http.StatusOK, KeywordResponseValidOK, body, err)

	case NameResponseValidAccepted:
		r.Set(NameResponseValidAccepted, http.StatusAccepted, KeywordResponseValidAccepted, body, err)

	case NameResponseValidNotModified:
		r.Set(NameResponseValidNotModified, http.StatusNotModified, KeywordResponseNotModified, body, err)

	case NameResponseErrorsForbidden:
		r.Set(NameResponseErrorsForbidden, http.StatusForbidden, KeywordResponseErrorsForbidden, body, err)

	case NameResponseErrorsUnauthorized:
		r.Set(NameResponseErrorsUnauthorized, http.StatusUnauthorized, KeywordResponseErrorsUnauthorized, body, err)

	case NameResponseErrorsNotFound:
		r.Set(NameResponseErrorsNotFound, http.StatusNotFound, KeywordResponseErrorsNotFound, body, err)

	case NameResponseErrorsInvalidQuery:
		r.Set(NameResponseErrorsInvalidQuery, http.StatusBadRequest, KeywordResponseErrorsInvalidQuery, body, err)

	case NameResponseErrorsRequestTooLarge:
		r.Set(NameResponseErrorsRequestTooLarge, http.StatusRequestEntityTooLarge, KeywordResponseErrorsRequestTooLarge, body, err)

	case NameResponseErrorsDecode:
		r.Set(NameResponseErrorsDecode, http.StatusBadRequest, KeywordResponseErrorsDecode, body, err)

	case NameResponseErrorsValidate:
		r.Set(NameResponseErrorsValidate, http.StatusBadRequest, KeywordResponseErrorsValidate, body, err)

	case NameResponseErrorsRateLimit:
		r.Set(NameResponseErrorsRateLimit, http.StatusTooManyRequests, KeywordResponseErrorsRateLimit, body, err)

	case NameResponseErrorsMethodNotAllowed:
		r.Set(NameResponseErrorsMethodNotAllowed, http.StatusMethodNotAllowed, KeywordResponseErrorsMethodNotAllowed, body, err)

	case NameResponseErrorsFullQueue:
		r.Set(NameResponseErrorsFullQueue, http.StatusServiceUnavailable, KeywordResponseErrorsFullQueue, body, err)

	case NameResponseErrorsShuttingDown:
		r.Set(NameResponseErrorsShuttingDown, http.StatusServiceUnavailable, KeywordResponseErrorsShuttingDown, body, err)

	case NameResponseErrorsServiceUnavailable:
		r.Set(NameResponseErrorsServiceUnavailable, http.StatusServiceUnavailable, KeywordResponseErrorsServiceUnavailable, body, err)

	case NameResponseErrorsInternal:
		fallthrough
	default:
		r.Set(NameResponseErrorsInternal, http.StatusInternalServerError, KeywordResponseErrorsInternal, body, err)
	}
}

type Result struct {
	Code    int
	Name    string
	Keyword string
	Err     error
	Body    interface{}
}

func (r *Result) Set(name string, statusCode int, keyword string, body interface{}, err error) {
	r.Name = name
	r.Code = statusCode
	r.Keyword = keyword
	r.Body = body
	if body != nil {
		r.Body = body
	} else if statusCode >= http.StatusBadRequest {
		r.Body = r.Err
		if err == nil {
			r.Err = errors.New(keyword)
		} else {
			r.Err = err
		}
	}
}
