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
