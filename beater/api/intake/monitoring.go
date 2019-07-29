package intake

import (
	"sync"

	"github.com/elastic/beats/libbeat/monitoring"

	"github.com/elastic/apm-server/beater/request"
)

var (
	m sync.Mutex

	serverMetrics = monitoring.Default.NewRegistry("apm-server.server", monitoring.PublishExpvar)
	counter       = func(s request.ResultID) *monitoring.Int {
		return monitoring.NewInt(serverMetrics, string(s))
	}

	resultIdToCounter = map[request.ResultID]*monitoring.Int{}
)

func ResultIdToMonitoringInt(name request.ResultID) *monitoring.Int {
	if i, ok := resultIdToCounter[name]; ok {
		return i
	}

	m.Lock()
	defer m.Unlock()
	ct := counter(name)
	resultIdToCounter[name] = ct
	return ct
}
