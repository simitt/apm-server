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
	"strings"

	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/monitoring"

	"github.com/elastic/apm-server/beater/headers"
	logs "github.com/elastic/apm-server/log"
)

const (
	mimeTypeAny             = "*/*"
	mimeTypeApplicationJSON = "application/json"
)

var (
	mimeTypesJSON = []string{mimeTypeAny, mimeTypeApplicationJSON}
)

// Context abstracts request and response information for http requests
type Context struct {
	Req    *http.Request
	Logger *logp.Logger

	w http.ResponseWriter

	statusCode    int
	err           interface{}
	stacktrace    string
	monitoringCts []*monitoring.Int
}

// Reset allows to reuse a context by removing all request specific information
func (c *Context) Reset(w http.ResponseWriter, r *http.Request) {
	c.Req = r

	c.w = w
	c.statusCode = http.StatusOK
	c.err = ""
	c.stacktrace = ""
	c.monitoringCts = nil
	c.Logger = nil
}

// Header returns the http.Header of the context's writer
func (c *Context) Header() http.Header {
	return c.w.Header()
}

// StatusCode returns the context's status code
func (c *Context) StatusCode() int {
	return c.statusCode
}

// Error returns the context's error
func (c *Context) Error() interface{} {
	return c.err
}

// Stacktrace returns the context's stacktrace
func (c *Context) Stacktrace() string {
	return c.stacktrace
}

// MonitoringCounts returns the monitoring integers collected through context processing
func (c *Context) MonitoringCounts() []*monitoring.Int {
	return c.monitoringCts
}

// AddStacktrace sets a stacktrace for the context
func (c *Context) AddStacktrace(stacktrace string) {
	c.stacktrace = stacktrace
}

// AddMonitoringCt adds request specific counter that should be increased during the request handling
func (c *Context) AddMonitoringCt(i *monitoring.Int) {
	c.monitoringCts = append(c.monitoringCts, i)
}

// Write sets response headers, and writes the body to the response writer.
// In case body is nil only the headers will be set.
// In case statusCode indicates an error response, the body is also set as error in the context.
func (c *Context) Write(body interface{}, statusCode int) {
	var err interface{}
	if statusCode >= http.StatusBadRequest {
		err = body
	}
	c.WriteWithError(body, err, statusCode)
}

// WriteWithError sets response headers, writes body to the response writer and sets the
// provided error in the context.
// In case body is nil only the headers will be set.
func (c *Context) WriteWithError(body, fullErr interface{}, statusCode int) {
	c.err = fullErr
	c.statusCode = statusCode
	c.w.Header().Set(headers.XContentTypeOptions, "nosniff")

	if body == nil {
		c.w.WriteHeader(c.statusCode)
		return
	}

	// necessary to keep current logic
	if statusCode >= http.StatusBadRequest {
		if b, ok := body.(string); ok {
			body = map[string]string{"error": b}
		}
	}

	var err error
	if c.acceptJSON() {
		c.w.Header().Set(headers.ContentType, "application/json")
		c.w.WriteHeader(c.statusCode)
		err = c.writeJSON(body, true)
	} else {
		c.w.Header().Set(headers.ContentType, "text/plain; charset=utf-8")
		c.w.WriteHeader(c.statusCode)
		err = c.writePlain(body)
	}
	if err != nil {
		c.errOnWrite(err)
	}
}

func (c *Context) acceptJSON() bool {
	acceptHeader := c.Req.Header.Get(headers.Accept)
	for _, s := range mimeTypesJSON {
		if strings.Contains(acceptHeader, s) {
			return true
		}
	}
	return false
}

func (c *Context) writeJSON(body interface{}, pretty bool) error {
	enc := json.NewEncoder(c.w)
	if pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(body)
}

func (c *Context) writePlain(body interface{}) error {
	if b, ok := body.(string); ok {
		_, err := c.w.Write([]byte(b + "\n"))
		return err
	}
	// unexpected behavior to return json but changing this would be breaking
	return c.writeJSON(body, false)
}

func (c *Context) errOnWrite(err error) {
	if c.Logger == nil {
		c.Logger = logp.NewLogger(logs.Response)
	}
	c.Logger.Errorw("write response", "error", err)
}
