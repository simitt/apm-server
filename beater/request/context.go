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
	Req                  *http.Request
	Logger               *logp.Logger
	TokenSet, Authorized bool

	w http.ResponseWriter

	statusCode int
	err        interface{}
	stacktrace string
	monitoring string
}

// Set allows to reuse a context by removing all request specific information
func (c *Context) Reset(w http.ResponseWriter, r *http.Request) {
	c.Req = r

	c.w = w
	c.statusCode = http.StatusOK
	c.err = ""
	c.stacktrace = ""
	c.monitoring = ""
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
func (c *Context) Monitoring() string {
	return c.monitoring
}

// AddStacktrace sets a stacktrace for the context
func (c *Context) AddStacktrace(stacktrace string) {
	c.stacktrace = stacktrace
}

// Write sets response headers, and writes the body to the response writer.
// In case body is nil only the headers will be set.
// In case statusCode indicates an error response, the body is also set as error in the context.
func (c *Context) Write(r *Result) {
	c.err = r.Err
	c.statusCode = r.Code
	c.monitoring = r.Name

	c.w.Header().Set(headers.XContentTypeOptions, "nosniff")

	body := r.Body

	if body == nil {
		body = r.Err
	}

	// necessary to keep current logic
	if r.Code >= http.StatusBadRequest {
		if b, ok := body.(string); ok {
			body = map[string]string{"error": b}
		}
	}

	if body == nil {
		c.w.WriteHeader(c.statusCode)
		return
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
