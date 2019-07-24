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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"

	"github.com/pkg/errors"

	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/monitoring"

	"github.com/elastic/apm-server/beater/headers"
)

func TestContext_Reset(t *testing.T) {
	w1 := httptest.NewRecorder()
	w1.WriteHeader(http.StatusServiceUnavailable)
	w2 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2 := httptest.NewRequest(http.MethodHead, "/new", nil)

	mockRegistry := monitoring.Default.NewRegistry("test.apm-server", monitoring.PublishExpvar)

	c := Context{
		Req: r1, w: w1,
		Logger:     logp.NewLogger(""),
		statusCode: http.StatusServiceUnavailable, err: errors.New("foo"),
		stacktrace: "bar", monitoringCts: []*monitoring.Int{monitoring.NewInt(mockRegistry, "xyz")},
	}
	c.Reset(w2, r2)
	assert.Equal(t, http.MethodHead, c.Req.Method)
	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, http.StatusOK, c.statusCode)
	assert.Empty(t, c.Logger)
	assert.Empty(t, c.err)
	assert.Empty(t, c.stacktrace)
	assert.Empty(t, c.monitoringCts)
}

func TestContext_Write(t *testing.T) {
	t.Run("StatusCodeOK", func(t *testing.T) {
		c, w := mockContextAccept("*/*")
		body := map[string]interface{}{"xyz": "bar"}

		c.Write(body, http.StatusOK)

		testHeader(t, c, "application/json")
		testProperties(t, c, nil, http.StatusOK)
		var b map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b))
		assert.Equal(t, body, b)
	})

	t.Run("StatusError", func(t *testing.T) {
		c, w := mockContextAccept("*/*")
		body := map[string]interface{}{"xyz": "bar"}

		c.Write(body, http.StatusBadRequest)

		testHeader(t, c, "application/json")
		testProperties(t, c, body, http.StatusBadRequest)
		var b map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b))
		assert.Equal(t, body, b)
	})
}

func TestContext_WriteWithError(t *testing.T) {
	t.Run("AcceptJSON", func(t *testing.T) {
		c, w := mockContextAccept("*/*")
		err := map[string]interface{}{"foo": "bar"}
		body := map[string]interface{}{"xyz": "bar"}

		c.WriteWithError(body, err, http.StatusBadGateway)

		testHeader(t, c, "application/json")
		testProperties(t, c, err, http.StatusBadGateway)
		var b map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b))
		assert.Equal(t, body, b)
	})

	t.Run("AcceptPlainJSON", func(t *testing.T) {
		c, w := mockContextAccept("application/text")
		err := map[string]interface{}{"foo": "bar"}
		body := map[string]interface{}{"xyz": "bar"}

		c.WriteWithError(body, err, http.StatusBadGateway)

		testHeader(t, c, "text/plain; charset=utf-8")
		testProperties(t, c, err, http.StatusBadGateway)
		assert.Equal(t, `{"xyz":"bar"}`+"\n", w.Body.String())
	})

	t.Run("AcceptPlainString", func(t *testing.T) {
		c, w := mockContextAccept("application/text")
		err := "foo"
		body := "bar"

		c.WriteWithError(body, err, http.StatusBadRequest)

		testHeader(t, c, "text/plain; charset=utf-8")
		testProperties(t, c, err, http.StatusBadRequest)
		assert.Equal(t, `{"error":"bar"}`+"\n", w.Body.String())
	})

	t.Run("EmptyBody", func(t *testing.T) {
		c, w := mockContextAccept("*/*")
		err := "foo"

		c.WriteWithError(nil, err, http.StatusMultipleChoices)

		testHeaderXContentTypeOptions(t, c)
		testProperties(t, c, err, http.StatusMultipleChoices)
		assert.Empty(t, w.Body.String())
	})

}

func testProperties(t *testing.T, c *Context, err interface{}, statusCode int) {
	assert.Equal(t, statusCode, c.StatusCode())
	assert.Equal(t, err, c.Error())
}

func testHeaderXContentTypeOptions(t *testing.T, c *Context) {
	assert.Equal(t, "nosniff", c.w.Header().Get(headers.XContentTypeOptions))
}

func testHeader(t *testing.T, c *Context, expected string) {
	assert.Equal(t, expected, c.w.Header().Get(headers.ContentType))
	testHeaderXContentTypeOptions(t, c)
}

func mockContextAccept(accept string) (*Context, *httptest.ResponseRecorder) {
	c := &Context{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodHead, "/", nil)
	r.Header.Set(headers.Accept, accept)
	c.Reset(w, r)
	return c, w
}
