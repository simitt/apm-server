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

package model

import (
	"github.com/elastic/apm-server/transform"
	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestParseJfr(t *testing.T) {
	profile, err := os.Open("../testdata/profile/profile.jfr")
	if err != nil {
		t.Error(err)
	}
	defer func() {
		if err := profile.Close(); err != nil {
			t.Error(err)
		}
	}()

	jfrProfile, err := ParseJfrProfile(profile)
	if err != nil {
		t.Error(err)
	}
	assert.Equal(t, int64(1613997069036000000), jfrProfile.startNanos)
	assert.Equal(t, 2, len(jfrProfile.samples))
	assert.Equal(t, 2, len(jfrProfile.stackTraces))

	event := JfrProfileEvent{
		Metadata: Metadata{Service: Service{Name: "myService", Environment: "test"}},
		Profile:  jfrProfile,
	}

	flameGraph := NewFlameGraph(&event)
	assert.NotEmpty(t, flameGraph.children)
	flatFlameGraph := NewFlatFlameGraph(flameGraph)
	assert.NotEmpty(t, flatFlameGraph.samples)
	assert.Greater(t, flatFlameGraph.samples[0], int64(0))

	events := event.appendBeatEvents(&transform.Config{DataStreams: true}, []beat.Event{})
	assert.Equal(t, 1, len(events))
}
