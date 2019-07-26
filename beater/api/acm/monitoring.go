package acm

import (
	"github.com/elastic/apm-server/beater/request"
	"github.com/elastic/beats/libbeat/monitoring"
)

var (
	serverMetrics = monitoring.Default.NewRegistry("apm-server.server.acm", monitoring.PublishExpvar)
	counter       = func(s string) *monitoring.Int {
		return monitoring.NewInt(serverMetrics, s)
	}

	//TODO: add more monitoring counters
	mapping = map[string]*monitoring.Int{
		request.NameRequestCount: counter(request.NameRequestCount),
	}
)

func MonitoringNameToInt(name string) *monitoring.Int {
	if i, ok := mapping[name]; ok {
		return i
	}
	return nil
}
