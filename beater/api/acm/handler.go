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

package acm

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/elastic/beats/libbeat/common"

	"github.com/pkg/errors"

	"github.com/elastic/apm-server/agentcfg"
	"github.com/elastic/apm-server/beater/config"
	"github.com/elastic/apm-server/beater/headers"
	"github.com/elastic/apm-server/beater/request"
	"github.com/elastic/apm-server/convert"
	"github.com/elastic/apm-server/kibana"
)

const (
	errMaxAgeDuration = 5 * time.Minute

	errMsgInvalidQuery               = "invalid query"
	errMsgKibanaDisabled             = "disabled Kibana configuration"
	errMsgKibanaVersionNotCompatible = "not a compatible Kibana version"
	errMsgMethodUnsupported          = "method not supported"
	errMsgNoKibanaConnection         = "unable to retrieve connection to Kibana"
	errMsgServiceUnavailable         = "service unavailable"
)

var (
	minKibanaVersion = common.MustNewVersion("7.3.0")
	errCacheControl  = fmt.Sprintf("max-age=%v, must-revalidate", errMaxAgeDuration.Seconds())
)

func Handler(kbClient kibana.Client, config *config.AgentConfig) request.Handler {
	cacheControl := fmt.Sprintf("max-age=%v, must-revalidate", config.Cache.Expiration.Seconds())
	fetcher := agentcfg.NewFetcher(kbClient, config.Cache.Expiration)

	return func(c *request.Context) {
		// error handling
		c.Header().Set(headers.CacheControl, errCacheControl)
		if valid, fullMsg := validateKbClient(kbClient); !valid {
			c.WriteWithError(extractInternalError(fullMsg, c.TokenSet))
			return
		}

		query, queryErr := buildQuery(c.Req)
		if queryErr != nil {
			c.WriteWithError(extractQueryError(queryErr.Error(), c.TokenSet))
			return
		}

		cfg, upstreamEtag, err := fetcher.Fetch(query, nil)
		if err != nil {
			c.WriteWithError(extractInternalError(err.Error(), c.TokenSet))
			return
		}

		// configuration successfully fetched
		c.Header().Set(headers.CacheControl, cacheControl)
		etag := fmt.Sprintf("\"%s\"", upstreamEtag)
		c.Header().Set(headers.Etag, etag)
		if etag == c.Req.Header.Get(headers.IfNoneMatch) {
			c.Write(nil, http.StatusNotModified)
		} else {
			c.Write(cfg, http.StatusOK)
		}
	}
}

func validateKbClient(client kibana.Client) (bool, string) {
	if client == nil {
		return false, errMsgKibanaDisabled
	}
	if !client.Connected() {
		return false, errMsgNoKibanaConnection
	}
	if supported, _ := client.SupportsVersion(minKibanaVersion); !supported {
		version, _ := client.GetVersion()

		return false, fmt.Sprintf("%s: min version %+v, configured version %+v",
			errMsgKibanaVersionNotCompatible, minKibanaVersion, version.String())
	}
	return true, ""
}

// Returns (zero, error) if request body can't be unmarshalled or service.name is missing
// Returns (zero, zero) if request method is not GET or POST
func buildQuery(r *http.Request) (query agentcfg.Query, err error) {
	switch r.Method {
	case http.MethodPost:
		err = convert.FromReader(r.Body, &query)
	case http.MethodGet:
		params := r.URL.Query()
		query = agentcfg.NewQuery(
			params.Get(agentcfg.ServiceName),
			params.Get(agentcfg.ServiceEnv),
		)
	default:
		err = errors.Errorf("%s: %s", errMsgMethodUnsupported, r.Method)
	}

	if err == nil && query.Service.Name == "" {
		err = errors.New(agentcfg.ServiceName + " is required")
	}
	return
}

func extractInternalError(msg string, withAuth bool) (string, string, int) {
	var shortMsg = errMsgServiceUnavailable
	switch {
	case msg == errMsgKibanaDisabled || msg == errMsgNoKibanaConnection:
		shortMsg = msg
	case strings.Contains(msg, errMsgKibanaVersionNotCompatible):
		shortMsg = errMsgKibanaVersionNotCompatible
	case strings.Contains(msg, agentcfg.ErrMsgSendToKibanaFailed):
		shortMsg = agentcfg.ErrMsgSendToKibanaFailed
	case strings.Contains(msg, agentcfg.ErrMsgMultipleChoices):
		shortMsg = agentcfg.ErrMsgMultipleChoices
	case strings.Contains(msg, agentcfg.ErrMsgReadKibanaResponse):
		shortMsg = agentcfg.ErrMsgReadKibanaResponse
	}
	return authErrMsg(msg, shortMsg, withAuth), msg, http.StatusServiceUnavailable
}

func extractQueryError(msg string, withAuth bool) (string, string, int) {
	if strings.Contains(msg, errMsgMethodUnsupported) {
		return authErrMsg(msg, errMsgMethodUnsupported, withAuth), msg, http.StatusMethodNotAllowed
	}
	return authErrMsg(msg, errMsgInvalidQuery, withAuth), msg, http.StatusBadRequest
}

func authErrMsg(fullMsg, shortMsg string, withAuth bool) string {
	if withAuth {
		return fullMsg
	}
	return shortMsg
}
