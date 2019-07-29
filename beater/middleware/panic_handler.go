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
	"fmt"
	"runtime/debug"

	"github.com/elastic/apm-server/beater/request"
)

const keywordPanic = "panic handling request"

func PanicHandler() Middleware {
	return func(h request.Handler) request.Handler {
		return func(c *request.Context) {

			defer func() {
				if r := recover(); r != nil {
					var ok bool
					var err error
					if err, ok = r.(error); !ok {
						err = fmt.Errorf("internal server error %+v", r)
					}
					c.Result.SetFor(request.IdResponseErrorsInternal)
					c.Result.Stacktrace = string(debug.Stack())
					c.Result.Keyword = keywordPanic
					c.Result.Body = keywordPanic
					c.Result.Err = err
				}
			}()
			h(c)
		}
	}
}
