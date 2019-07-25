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

package asset

import (
	"net/http"
	"strings"

	"go.elastic.co/apm"

	"github.com/elastic/apm-server/beater/config"
	"github.com/elastic/apm-server/beater/request"
	"github.com/elastic/apm-server/decoder"
	"github.com/elastic/apm-server/processor/asset"
	"github.com/elastic/apm-server/publish"
	"github.com/elastic/apm-server/transform"
	"github.com/elastic/apm-server/utility"
)

type Handler struct {
	requestDecoder decoder.ReqDecoder
	processor      asset.Processor
	tconfig        transform.Config
}

func NewHandler(dec decoder.ReqDecoder, processor asset.Processor, cfg transform.Config) *Handler {
	return &Handler{requestDecoder: dec, processor: processor, tconfig: cfg}
}

func (h *Handler) Handle(beaterConfig *config.Config, report publish.Reporter) request.Handler {
	return func(c *request.Context) {
		h.processRequest(c.Req, report).WriteTo(c)
	}
}

func (h *Handler) processRequest(r *http.Request, report publish.Reporter) request.Result {
	if r.Method != "POST" {
		return request.MethodNotAllowedResult
	}

	data, err := h.requestDecoder(r)
	if err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			return request.RequestTooLargeResult
		}
		return request.CannotDecodeResult(err)
	}

	if err = h.processor.Validate(data); err != nil {
		return request.CannotValidateResult(err)
	}

	metadata, transformables, err := h.processor.Decode(data)
	if err != nil {
		return request.CannotDecodeResult(err)
	}

	tctx := &transform.Context{
		RequestTime: utility.RequestTime(r.Context()),
		Config:      h.tconfig,
		Metadata:    *metadata,
	}

	req := publish.PendingReq{Transformables: transformables, Tcontext: tctx}
	ctx := r.Context()
	span, ctx := apm.StartSpan(ctx, "Send", "Reporter")
	defer span.End()
	req.Trace = !span.Dropped()

	if err = report(ctx, req); err != nil {
		if err == publish.ErrChannelClosed {
			return request.ServerShuttingDownResult(err)
		}
		return request.FullQueueResult(err)
	}

	return request.AcceptedResult
}
