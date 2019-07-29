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

	"github.com/elastic/beats/libbeat/monitoring"

	"github.com/elastic/apm-server/beater/request"
)

func MonitoringHandler(fn func(id request.ResultID) *monitoring.Int) Middleware {
	return func(h request.Handler) request.Handler {
		inc := func(counter *monitoring.Int) {
			if counter == nil {
				return
			}
			counter.Inc()
		}
		return func(c *request.Context) {
			inc(fn(request.IdRequestCount))

			h(c)

			inc(fn(request.IdResponseCount))
			if c.Result.StatusCode >= http.StatusBadRequest {
				inc(fn(request.IdResponseErrorsCount))
			} else {
				inc(fn(request.IdResponseValidCount))
			}

			inc(fn(c.Result.Id))
		}

	}
}
