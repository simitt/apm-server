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

package api

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/elastic/apm-server/beater/api/acm"
	"github.com/elastic/apm-server/beater/api/asset"
	"github.com/elastic/apm-server/beater/api/intake"
	"github.com/elastic/apm-server/beater/api/root"
	"github.com/elastic/apm-server/beater/config"
	"github.com/elastic/apm-server/beater/middleware"
	"github.com/elastic/apm-server/beater/request"
	"github.com/elastic/apm-server/decoder"
	"github.com/elastic/apm-server/kibana"
	"github.com/elastic/apm-server/model"
	passet "github.com/elastic/apm-server/processor/asset"
	"github.com/elastic/apm-server/processor/asset/sourcemap"
	"github.com/elastic/apm-server/processor/stream"
	"github.com/elastic/apm-server/publish"
	"github.com/elastic/apm-server/transform"
)

const (
	RootURL = "/"

	AgentConfigURL = "/config/v1/agents"

	// intake v2
	BackendURL = "/intake/v2/events"
	RumURL     = "/intake/v2/rum/events"

	// assets
	SourcemapsURL = "/assets/v1/sourcemaps"

	slash = "/"

	burstMultiplier = 3
)

type routeType struct {
	wrappingHandler     func(*config.Config, request.Handler) request.Handler
	configurableDecoder func(*config.Config, decoder.ReqDecoder) decoder.ReqDecoder
	transformConfig     func(*config.Config) (*transform.Config, error)
}

var IntakeRoutes = map[string]intakeRoute{
	BackendURL: backendRoute,
	RumURL:     rumRoute,
}

var AssetRoutes = map[string]assetRoute{
	SourcemapsURL: {sourcemapRouteType, sourcemap.Processor, sourcemapUploadDecoder},
}

var (
	backendRoute = intakeRoute{
		routeType{
			backendHandler,
			systemMetadataDecoder,
			func(*config.Config) (*transform.Config, error) { return &transform.Config{}, nil },
		},
	}
	rumRoute = intakeRoute{
		routeType{
			rumHandler,
			userMetaDataDecoder,
			rumTransformConfig,
		},
	}

	sourcemapRouteType = routeType{
		sourcemapHandler,
		systemMetadataDecoder,
		rumTransformConfig,
	}

	sourcemapUploadDecoder = func(beaterConfig *config.Config) decoder.ReqDecoder {
		return decoder.DecodeSourcemapFormData
	}
)

func middlewareFor(r *http.Request, h request.Handler) request.Handler {
	return middleware.LogHandler(monitoringHandler(r)(middleware.PanicHandler(h)))
}

func monitoringHandler(r *http.Request) func(request.Handler) request.Handler {
	switch strings.TrimSuffix(r.URL.Path, slash) {
	case AgentConfigURL:
		return acm.MonitoringHandler
	case BackendURL:
		fallthrough
	case RumURL:
		return intake.MonitoringHandler
	case SourcemapsURL:
		return asset.MonitoringHandler
	default:
		return root.MonitoringHandler
	}
}

func backendHandler(beaterConfig *config.Config, h request.Handler) request.Handler {
	return middleware.RequestTimeHandler(
		middleware.AuthHandler(beaterConfig.SecretToken, h))
}

func rumHandler(beaterConfig *config.Config, h request.Handler) request.Handler {
	return middleware.KillSwitchHandler(beaterConfig.RumConfig.IsEnabled(),
		middleware.RequestTimeHandler(
			middleware.CorsHandler(beaterConfig.RumConfig.AllowOrigins, h)))
}

func sourcemapHandler(beaterConfig *config.Config, h request.Handler) request.Handler {
	return middleware.KillSwitchHandler(beaterConfig.RumConfig.IsEnabled() && beaterConfig.RumConfig.SourceMapping.IsEnabled(),
		middleware.AuthHandler(beaterConfig.SecretToken, h))
}

func agentHandler(beaterConfig *config.Config) request.Handler {
	var kbClient kibana.Client
	if beaterConfig.Kibana.Enabled() {
		kbClient = kibana.NewConnectingClient(beaterConfig.Kibana)
	}
	return middleware.KillSwitchHandler(kbClient != nil,
		middleware.AuthHandler(beaterConfig.SecretToken,
			acm.Handler(kbClient, beaterConfig.AgentConfig, beaterConfig.SecretToken)))
}

func systemMetadataDecoder(beaterConfig *config.Config, d decoder.ReqDecoder) decoder.ReqDecoder {
	return decoder.DecodeSystemData(d, beaterConfig.AugmentEnabled)
}

func userMetaDataDecoder(beaterConfig *config.Config, d decoder.ReqDecoder) decoder.ReqDecoder {
	return decoder.DecodeUserData(d, beaterConfig.AugmentEnabled)
}

func rumTransformConfig(beaterConfig *config.Config) (*transform.Config, error) {
	smapper, err := beaterConfig.RumConfig.MemoizedSmapMapper()
	if err != nil {
		return nil, err
	}
	return &transform.Config{
		SmapMapper:          smapper,
		LibraryPattern:      regexp.MustCompile(beaterConfig.RumConfig.LibraryPattern),
		ExcludeFromGrouping: regexp.MustCompile(beaterConfig.RumConfig.ExcludeFromGrouping),
	}, nil
}

type assetRoute struct {
	routeType
	passet.Processor
	topLevelRequestDecoder func(*config.Config) decoder.ReqDecoder
}

func (r *assetRoute) Handler(p passet.Processor, beaterConfig *config.Config, report publish.Reporter) (request.Handler, error) {
	cfg, err := r.transformConfig(beaterConfig)
	if err != nil {
		return nil, err
	}
	h := asset.NewHandler(r.configurableDecoder(beaterConfig, r.topLevelRequestDecoder(beaterConfig)), p, *cfg)
	return r.wrappingHandler(beaterConfig, h.Handle(beaterConfig, report)), nil
}

type intakeRoute struct {
	routeType
}

func (r intakeRoute) Handler(url string, c *config.Config, report publish.Reporter) (request.Handler, error) {
	reqDecoder := r.configurableDecoder(
		c,
		func(*http.Request) (map[string]interface{}, error) { return map[string]interface{}{}, nil },
	)
	cfg, err := r.transformConfig(c)
	if err != nil {
		return nil, err
	}
	var cache *intake.RlCache
	if url == RumURL {
		cache, err = intake.NewRlCache(c.RumConfig.EventRate.LruSize, c.RumConfig.EventRate.Limit, burstMultiplier)
		if err != nil {
			return nil, err
		}
	}

	h := intake.NewHandler(
		reqDecoder,
		&stream.Processor{
			Tconfig:      *cfg,
			Mconfig:      model.Config{Experimental: c.Mode == config.ModeExperimental},
			MaxEventSize: c.MaxEventSize,
		},
		cache)

	return r.wrappingHandler(c, h.Handle(c, report)), nil
}
