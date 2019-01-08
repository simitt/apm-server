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

package metadata

import (
	"fmt"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/processors"
)

// init registers the add_apm_metadata processor
func init() {
	processors.RegisterPlugin("add_apm_metadata", newAddApmMetadata)
}

type addApmMetadata struct{}

func newAddApmMetadata(c *common.Config) (processors.Processor, error) {
	return &addApmMetadata{}, nil
}

// Run sets agent.* and host.*, which the libbeat publisher previously set
// libbeat expects the agent/host to be the one which the "beat" (apm-server) is running on
// APM wants those values to reflect the location where the data is gathered
// Instead, observer.* will hold the apm-server data.
func (p *addApmMetadata) Run(event *beat.Event) (*beat.Event, error) {
	moveField(event, "agent", "context.service.agent")
	moveField(event, "host", "context.system")
	event.Delete("host.name")
	return event, nil
}

// TODO: implement in MapStr to eliminate copies?
func moveField(event *beat.Event, to string, from string) {
	fromVal, err := event.Fields.GetValue(from)
	if err != nil {
		fmt.Println(from, err)
		return
	}
	if val, ok := fromVal.(common.MapStr); ok {
		event.Fields.Put(to, val)
	}

	event.Delete(from)
}

func (p *addApmMetadata) String() string {
	return "add_apm_metadata"
}
