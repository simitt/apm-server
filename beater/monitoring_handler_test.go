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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/elastic/beats/libbeat/monitoring"

	"github.com/elastic/apm-server/beater/request"
)

func TestMonitoringHandler_ACM(t *testing.T) {

	t.Run("Error", func(t *testing.T) {
		testResetCounter()
		c := setupContext("/config/v1/agents?service.name=xyz")
		monitoringHandler(mockHandler403)(c)
		testCounter(t, map[*monitoring.Int]int64{requestCounter: 1})
	})
	t.Run("Accepted", func(t *testing.T) {
		testResetCounter()
		c := setupContext("/config/v1/agents")
		monitoringHandler(mockHandler202)(c)
		testCounter(t, map[*monitoring.Int]int64{requestCounter: 1})
	})
	t.Run("Idle", func(t *testing.T) {
		testResetCounter()
		c := setupContext("/config/v1/agents/")
		monitoringHandler(mockHandlerIdle)(c)
		testCounter(t, map[*monitoring.Int]int64{requestCounter: 1})
	})
}
func TestMonitoringHandler_IntakeBackend(t *testing.T) {
	t.Run("Error", func(t *testing.T) {
		testResetCounter()
		c := setupContext("/intake/v2/events")
		monitoringHandler(mockHandler403)(c)
		testCounter(t, map[*monitoring.Int]int64{requestCounter: 1,
			responseCounter: 1, responseErrors: 1, forbiddenCounter: 1})
	})
	t.Run("Accepted", func(t *testing.T) {
		testResetCounter()
		c := setupContext("/intake/v2/events/")
		monitoringHandler(mockHandler202)(c)
		testCounter(t, map[*monitoring.Int]int64{requestCounter: 1,
			responseCounter: 1, responseSuccesses: 1, responseOk: 1})
	})
	t.Run("Idle", func(t *testing.T) {
		testResetCounter()
		c := setupContext("/intake/v2/events")
		monitoringHandler(mockHandlerIdle)(c)
		testCounter(t, map[*monitoring.Int]int64{requestCounter: 1,
			responseCounter: 1, responseSuccesses: 1})
	})
}

func TestMonitoringHandler_IntakeRUM(t *testing.T) {
	t.Run("Error", func(t *testing.T) {
		testResetCounter()
		c := setupContext("/intake/v2/rum/events")
		monitoringHandler(mockHandler403)(c)
		testCounter(t, map[*monitoring.Int]int64{requestCounter: 1,
			responseCounter: 1, responseErrors: 1, forbiddenCounter: 1})
	})
	t.Run("Accepted", func(t *testing.T) {
		testResetCounter()
		c := setupContext("/intake/v2/rum/events/")
		monitoringHandler(mockHandler202)(c)
		testCounter(t, map[*monitoring.Int]int64{requestCounter: 1,
			responseCounter: 1, responseSuccesses: 1, responseOk: 1})
	})
	t.Run("Idle", func(t *testing.T) {
		testResetCounter()
		c := setupContext("/intake/v2/rum/events")
		monitoringHandler(mockHandlerIdle)(c)
		testCounter(t, map[*monitoring.Int]int64{requestCounter: 1,
			responseCounter: 1, responseSuccesses: 1})
	})
}

func TestMonitoringHandler_Root(t *testing.T) {
	t.Run("Error", func(t *testing.T) {
		testResetCounter()
		c := setupContext("/")
		monitoringHandler(mockHandler403)(c)
		testCounter(t, map[*monitoring.Int]int64{requestCounter: 1,
			responseCounter: 1, responseErrors: 1, forbiddenCounter: 1})
	})
	t.Run("Accepted", func(t *testing.T) {
		testResetCounter()
		c := setupContext("/")
		monitoringHandler(mockHandler202)(c)
		testCounter(t, map[*monitoring.Int]int64{requestCounter: 1,
			responseCounter: 1, responseSuccesses: 1, responseOk: 1})
	})
	t.Run("Idle", func(t *testing.T) {
		testResetCounter()
		c := setupContext("/")
		monitoringHandler(mockHandlerIdle)(c)
		testCounter(t, map[*monitoring.Int]int64{requestCounter: 1,
			responseCounter: 1, responseSuccesses: 1})
	})
}

func TestMonitoringHandler_Asset(t *testing.T) {
	t.Run("Error", func(t *testing.T) {
		testResetCounter()
		c := setupContext("/assets/v1/sourcemaps/")
		monitoringHandler(mockHandler403)(c)
		testCounter(t, map[*monitoring.Int]int64{requestCounter: 1,
			responseCounter: 1, responseErrors: 1, forbiddenCounter: 1})
	})
	t.Run("Accepted", func(t *testing.T) {
		testResetCounter()
		c := setupContext("/assets/v1/sourcemaps")
		monitoringHandler(mockHandler202)(c)
		testCounter(t, map[*monitoring.Int]int64{requestCounter: 1,
			responseCounter: 1, responseSuccesses: 1, responseOk: 1})
	})
	t.Run("Idle", func(t *testing.T) {
		testResetCounter()
		c := setupContext("/assets/v1/sourcemaps")
		monitoringHandler(mockHandlerIdle)(c)
		testCounter(t, map[*monitoring.Int]int64{requestCounter: 1,
			responseCounter: 1, responseSuccesses: 1})
	})
}

func testCounter(t *testing.T, ctrs map[*monitoring.Int]int64) {
	for idx, ct := range testGetCounter() {
		actual := ct.Get()
		expected := int64(0)
		if val, included := ctrs[ct]; included {
			expected = val
		}
		assert.Equal(t, expected, actual, fmt.Sprintf("Idx: %d", idx))
	}
}

func setupContext(path string) *request.Context {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	c := &request.Context{}
	c.Reset(w, r)
	return c
}

func mockHandler403(c *request.Context) {
	c.AddMonitoringCt(forbiddenCounter)
	c.Write(nil, http.StatusForbidden)
}

func mockHandler202(c *request.Context) {
	c.AddMonitoringCt(responseOk)
	c.Write(nil, http.StatusAccepted)
}

func mockHandlerIdle(c *request.Context) {}

func testResetCounter() {
	for _, ct := range testGetCounter() {
		ct.Set(0)
	}
}

func testGetCounter() []*monitoring.Int {
	return []*monitoring.Int{
		requestCounter,
		responseCounter,
		responseErrors,
		responseSuccesses,
		responseOk,
		responseAccepted,
		internalErrorCounter,
		forbiddenCounter,
		requestTooLargeCounter,
		decodeCounter,
		validateCounter,
		rateLimitCounter,
		methodNotAllowedCounter,
		fullQueueCounter,
		serverShuttingDownCounter,
		unauthorizedCounter,
	}
}
