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
	"github.com/elastic/apm-server/beater/request"
)

const (
	slash = "/"
)

func MonitoringHandler(h request.Handler) request.Handler {
	return func(c *request.Context) {
		h(c)

		//switch strings.TrimSuffix(c.Req.URL.Path, slash) {
		//case agentConfigURL:
		//	// do not monitor yet
		//	requestCounter.Inc()
		//default:
		//	//TODO: use extra monitoring counters for rootHandler and assetHandler
		//	requestCounter.Inc()
		//	responseCounter.Inc()
		//	if c.StatusCode() >= http.StatusBadRequest {
		//		responseErrors.Inc()
		//	} else {
		//		responseSuccesses.Inc()
		//	}
		//
		//	for _, ct := range c.MonitoringCounts() {
		//		ct.Inc()
		//	}
		//}
	}
}
