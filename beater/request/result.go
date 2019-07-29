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
)

const (
	IdUnset ResultID = "unset"

	IdRequestCount        ResultID = "request.count"
	IdResponseCount       ResultID = "response.count"
	IdResponseErrorsCount ResultID = "response.errors.count"
	IdResponseValidCount  ResultID = "response.valid.count"

	IdResponseValidOK          ResultID = "response.valid.ok"
	IdResponseValidAccepted    ResultID = "response.valid.accepted"
	IdResponseValidNotModified ResultID = "response.valid.notmodified"

	IdResponseErrorsForbidden          ResultID = "response.errors.forbidden"
	IdResponseErrorsUnauthorized       ResultID = "response.errors.unauthorized"
	IdResponseErrorsNotFound           ResultID = "response.errors.notfound"
	IdResponseErrorsInvalidQuery       ResultID = "response.errors.invalidquery"
	IdResponseErrorsRequestTooLarge    ResultID = "response.errors.toolarge"
	IdResponseErrorsDecode             ResultID = "response.errors.decode"
	IdResponseErrorsValidate           ResultID = "response.errors.validate"
	IdResponseErrorsRateLimit          ResultID = "response.errors.ratelimit"
	IdResponseErrorsMethodNotAllowed   ResultID = "response.errors.method"
	IdResponseErrorsFullQueue          ResultID = "response.errors.queue"
	IdResponseErrorsShuttingDown       ResultID = "response.errors.closed"
	IdResponseErrorsServiceUnavailable ResultID = "response.errors.unavailable"
	IdResponseErrorsInternal           ResultID = "response.errors.internal"

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

type ResultID string

type Result struct {
	Id         ResultID
	StatusCode int
	Keyword    string
	Body       interface{}
	Err        error
	Stacktrace string
}

func (r *Result) Reset() {
	r.Id = IdUnset
	r.StatusCode = http.StatusOK
	r.Keyword = ""
	r.Body = nil
	r.Err = nil
	r.Stacktrace = ""
}

func (r *Result) SetFor(id ResultID) {
	if r == nil {
		return
	}
	r.Id = id

	switch id {
	case IdResponseValidOK:
		r.StatusCode = http.StatusOK
		r.Keyword = KeywordResponseValidOK

	case IdResponseValidAccepted:
		r.StatusCode = http.StatusAccepted
		r.Keyword = KeywordResponseValidAccepted

	case IdResponseValidNotModified:
		r.StatusCode = http.StatusNotModified
		r.Keyword = KeywordResponseNotModified

	case IdResponseErrorsForbidden:
		r.StatusCode = http.StatusForbidden
		r.Keyword = KeywordResponseErrorsForbidden

	case IdResponseErrorsUnauthorized:
		r.StatusCode = http.StatusUnauthorized
		r.Keyword = KeywordResponseErrorsUnauthorized

	case IdResponseErrorsNotFound:
		r.StatusCode = http.StatusNotFound
		r.Keyword = KeywordResponseErrorsNotFound

	case IdResponseErrorsInvalidQuery:
		r.StatusCode = http.StatusBadRequest
		r.Keyword = KeywordResponseErrorsInvalidQuery

	case IdResponseErrorsRequestTooLarge:
		r.StatusCode = http.StatusRequestEntityTooLarge
		r.Keyword = KeywordResponseErrorsRequestTooLarge

	case IdResponseErrorsDecode:
		r.StatusCode = http.StatusBadRequest
		r.Keyword = KeywordResponseErrorsDecode

	case IdResponseErrorsValidate:
		r.StatusCode = http.StatusBadRequest
		r.Keyword = KeywordResponseErrorsValidate

	case IdResponseErrorsRateLimit:
		r.StatusCode = http.StatusTooManyRequests
		r.Keyword = KeywordResponseErrorsRateLimit

	case IdResponseErrorsMethodNotAllowed:
		r.StatusCode = http.StatusMethodNotAllowed
		r.Keyword = KeywordResponseErrorsMethodNotAllowed

	case IdResponseErrorsFullQueue:
		r.StatusCode = http.StatusServiceUnavailable
		r.Keyword = KeywordResponseErrorsFullQueue

	case IdResponseErrorsShuttingDown:
		r.StatusCode = http.StatusServiceUnavailable
		r.Keyword = KeywordResponseErrorsShuttingDown

	case IdResponseErrorsServiceUnavailable:
		r.StatusCode = http.StatusServiceUnavailable
		r.Keyword = KeywordResponseErrorsServiceUnavailable

	case IdResponseErrorsInternal:
		fallthrough

	default:
		r.StatusCode = http.StatusInternalServerError
		r.Keyword = KeywordResponseErrorsInternal
	}
}

func (r *Result) Set(id ResultID, statusCode int, keyword string, body interface{}, err error) {
	if r == nil {
		return
	}
	r.Id = id
	r.StatusCode = statusCode
	r.Keyword = keyword
	r.Body = body
	r.Err = err
}

//func (r *Result) set(statusCode int, keyword string) {
//	r.Id = id
//	r.StatusCode = statusCode
//	r.Keyword = keyword
//	r.Body = body
//
//	//if body != nil {
//	//	r.Body = body
//	//} else if statusCode >= http.StatusBadRequest {
//	//	r.Body = r.Err
//	//	if err == nil {
//	//		r.Err = errors.New(keyword)
//	//	} else {
//	//		r.Err = err
//	//	}
//	//}
//}
