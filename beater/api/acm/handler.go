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

	"github.com/pkg/errors"

	"github.com/elastic/beats/libbeat/common"

	"github.com/elastic/apm-server/agentcfg"
	"github.com/elastic/apm-server/beater/config"
	"github.com/elastic/apm-server/beater/headers"
	"github.com/elastic/apm-server/beater/request"
	"github.com/elastic/apm-server/convert"
	"github.com/elastic/apm-server/kibana"
)

const (
	errMaxAgeDuration = 5 * time.Minute

	msgInvalidQuery               = "invalid query"
	msgKibanaDisabled             = "disabled Kibana configuration"
	msgKibanaVersionNotCompatible = "not a compatible Kibana version"
	msgMethodUnsupported          = "method not supported"
	msgNoKibanaConnection         = "unable to retrieve connection to Kibana"
	msgServiceUnavailable         = "service unavailable"

	keywordFetched  = "fetched"
	keywordModified = "not modified"
)

var (
	errMsgKibanaDisabled     = errors.New(msgKibanaDisabled)
	errMsgNoKibanaConnection = errors.New(msgNoKibanaConnection)
)

var (
	minKibanaVersion = common.MustNewVersion("7.3.0")
	errCacheControl  = fmt.Sprintf("max-age=%v, must-revalidate", errMaxAgeDuration.Seconds())
)

func Handler(kbClient kibana.Client, config *config.AgentConfig) request.Handler {
	cacheControl := fmt.Sprintf("max-age=%v, must-revalidate", config.Cache.Expiration.Seconds())
	fetcher := agentcfg.NewFetcher(kbClient, config.Cache.Expiration)

	return func(c *request.Context) {
		var result request.Result
		// error handling
		c.Header().Set(headers.CacheControl, errCacheControl)
		if valid := validateKbClient(kbClient, c.TokenSet, &result); !valid {
			c.Write(&result)
			return
		}

		query, queryErr := buildQuery(c.Req)
		if queryErr != nil {
			extractQueryError(queryErr, c.TokenSet, &result)
			c.Write(&result)
			return
		}

		cfg, upstreamEtag, err := fetcher.Fetch(query, nil)
		if err != nil {
			extractInternalError(err, c.TokenSet, &result)
			c.Write(&result)
			return
		}

		// configuration successfully fetched
		c.Header().Set(headers.CacheControl, cacheControl)
		etag := fmt.Sprintf("\"%s\"", upstreamEtag)
		c.Header().Set(headers.Etag, etag)
		if etag == c.Req.Header.Get(headers.IfNoneMatch) {
			result.Set(request.NameResponseValidNotModified, http.StatusNotModified, keywordModified, nil, nil)
		} else {
			result.Set(request.NameResponseValidOK, http.StatusOK, keywordFetched, cfg, nil)
		}
		c.Write(&result)
	}
}

//TODO: chose more finegranular names for results for monitoring
func validateKbClient(client kibana.Client, withAuth bool, r *request.Result) bool {
	if client == nil {
		r.Set(request.NameResponseErrorsServiceUnavailable, http.StatusServiceUnavailable, msgKibanaDisabled, msgKibanaDisabled, errMsgKibanaDisabled)
		return false
	}
	if !client.Connected() {
		r.Set(request.NameResponseErrorsServiceUnavailable, http.StatusServiceUnavailable, msgNoKibanaConnection, msgNoKibanaConnection, errMsgNoKibanaConnection)
		return false
	}
	if supported, _ := client.SupportsVersion(minKibanaVersion); !supported {
		version, _ := client.GetVersion()

		errMsg := fmt.Sprintf("%s: min version %+v, configured version %+v",
			msgKibanaVersionNotCompatible, minKibanaVersion, version.String())
		body := authErrMsg(errMsg, msgKibanaVersionNotCompatible, withAuth)
		r.Set(request.NameResponseErrorsServiceUnavailable, http.StatusServiceUnavailable, msgKibanaVersionNotCompatible, body, errors.New(errMsg))
		return false
	}
	return true
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
		err = errors.Errorf("%s: %s", msgMethodUnsupported, r.Method)
	}

	if err == nil && query.Service.Name == "" {
		err = errors.New(agentcfg.ServiceName + " is required")
	}
	return
}

func extractInternalError(err error, withAuth bool, r *request.Result) {
	msg := err.Error()
	switch {
	case strings.Contains(msg, agentcfg.ErrMsgSendToKibanaFailed):
		body := authErrMsg(msg, agentcfg.ErrMsgSendToKibanaFailed, withAuth)
		r.Set(request.NameResponseErrorsServiceUnavailable, http.StatusServiceUnavailable, agentcfg.ErrMsgSendToKibanaFailed, body, err)

	case strings.Contains(msg, agentcfg.ErrMsgMultipleChoices):
		body := authErrMsg(msg, agentcfg.ErrMsgMultipleChoices, withAuth)
		r.Set(request.NameResponseErrorsServiceUnavailable, http.StatusServiceUnavailable, agentcfg.ErrMsgMultipleChoices, body, err)

	case strings.Contains(msg, agentcfg.ErrMsgReadKibanaResponse):
		body := authErrMsg(msg, agentcfg.ErrMsgReadKibanaResponse, withAuth)
		r.Set(request.NameResponseErrorsServiceUnavailable, http.StatusServiceUnavailable, agentcfg.ErrMsgReadKibanaResponse, body, err)

	default:
		body := authErrMsg(msg, msgServiceUnavailable, withAuth)
		r.Set(request.NameResponseErrorsServiceUnavailable, http.StatusServiceUnavailable, msgServiceUnavailable, body, err)
	}
}

func extractQueryError(err error, withAuth bool, r *request.Result) {
	msg := err.Error()
	if strings.Contains(msg, msgMethodUnsupported) {
		body := authErrMsg(msg, msgMethodUnsupported, withAuth)
		r.Set(request.NameResponseErrorsMethodNotAllowed, http.StatusMethodNotAllowed, msgMethodUnsupported, body, err)
		return
	}
	body := authErrMsg(msg, msgInvalidQuery, withAuth)
	r.Set(request.NameResponseErrorsInvalidQuery, http.StatusBadRequest, msgInvalidQuery, body, err)
}

func authErrMsg(fullMsg, shortMsg string, withAuth bool) string {
	if withAuth {
		return fullMsg
	}
	return shortMsg
}
