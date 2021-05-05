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

package benchtest

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"runtime"
	"strings"
)

// TODO(axw) reuse apmservertest.Expvar, expose function(s) for fetching
// from APM Server given a URL.

type expvar struct {
	runtime.MemStats `json:"memstats"`
	LibbeatStats

	// UncompressedBytes holds the number of bytes of uncompressed
	// data that the server has read from the Elastic APM events
	// intake endpoint.
	//
	// TODO(axw) instrument the net/http.Transport to count bytes
	// transferred, so we can measure for OTLP and Jaeger too.
	// Alternatively, implement an in-memory reverse proxy that
	// does the same.
	UncompressedBytes int64 `json:"apm-server.decoder.uncompressed.bytes"`
}

type LibbeatStats struct {
	ActiveEvents int64 `json:"libbeat.output.events.active"`
	TotalEvents  int64 `json:"libbeat.output.events.total"`
}

func queryExpvar(out *expvar) error {
	url := *server + "/debug/vars"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	client := http.DefaultClient
	if strings.HasPrefix(url, "https") {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}
