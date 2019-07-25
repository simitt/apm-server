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

package middleware

import (
	"net/http"

	"github.com/gofrs/uuid"

	"github.com/elastic/beats/libbeat/logp"

	"github.com/elastic/apm-server/beater/headers"
	"github.com/elastic/apm-server/beater/request"
	logs "github.com/elastic/apm-server/log"
	"github.com/elastic/apm-server/utility"
)

func LogHandler(h request.Handler) request.Handler {
	logger := logp.NewLogger(logs.Request)

	return func(c *request.Context) {
		reqID, err := uuid.NewV4()
		if err != nil {
			request.InternalErrorResult(err).WriteTo(c)
		}

		reqLogger := logger.With(
			"request_id", reqID,
			"method", c.Req.Method,
			"URL", c.Req.URL,
			"content_length", c.Req.ContentLength,
			"remote_address", utility.RemoteAddr(c.Req),
			"user-agent", c.Req.Header.Get(headers.UserAgent))

		c.Logger = reqLogger
		h(c)

		keysAndValues := []interface{}{"response_code", c.StatusCode()}
		if c.StatusCode() >= http.StatusBadRequest {
			keysAndValues = append(keysAndValues, "error", c.Error())
			if c.Stacktrace() != "" {
				keysAndValues = append(keysAndValues, "stacktrace", c.Stacktrace())
			}
			reqLogger.Errorw("error handling request", keysAndValues...)
			return
		}

		//TODO: log body if log level is debug
		reqLogger.Infow("handled request", keysAndValues...)
	}
}