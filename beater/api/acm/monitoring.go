package acm

import (
	"github.com/elastic/apm-server/beater/api/intake"
	"github.com/elastic/apm-server/beater/request"
	"github.com/elastic/beats/libbeat/monitoring"
)

var (
	//TODO: change logic for acm specific monitoring counters
	//serverMetrics = monitoring.Default.NewRegistry("apm-server.server.acm", monitoring.PublishExpvar)
	//counter       = func(s request.ResultID) *monitoring.Int {
	//	return monitoring.NewInt(serverMetrics, string(s))
	//}

	// reflects current behavior
	countRequest = intake.ResultIdToMonitoringInt(request.IdRequestCount)

	mapping = map[request.ResultID]*monitoring.Int{
		request.IdRequestCount: countRequest,
	}
)

func ResultIdToMonitoringInt(id request.ResultID) *monitoring.Int {
	if i, ok := mapping[id]; ok {
		return i
	}
	return nil
}
