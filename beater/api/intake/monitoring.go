package intake

import (
	"github.com/elastic/beats/libbeat/monitoring"

	"github.com/elastic/apm-server/beater/request"
)

var (
	serverMetrics = monitoring.Default.NewRegistry("apm-server.server", monitoring.PublishExpvar)
	counter       = func(s string) *monitoring.Int {
		return monitoring.NewInt(serverMetrics, s)
	}

	mapping = map[string]*monitoring.Int{
		request.NameRequestCount:        counter(request.NameRequestCount),
		request.NameResponseCount:       counter(request.NameResponseCount),
		request.NameResponseErrorsCount: counter(request.NameResponseErrorsCount),
		request.NameResponseValidCount:  counter(request.NameResponseValidCount),

		request.NameResponseValidOK:       counter(request.NameResponseValidOK),
		request.NameResponseValidAccepted: counter(request.NameResponseValidAccepted),

		request.NameResponseErrorsForbidden:        counter(request.NameResponseErrorsForbidden),
		request.NameResponseErrorsUnauthorized:     counter(request.NameResponseErrorsUnauthorized),
		request.NameResponseErrorsNotFound:         counter(request.NameResponseErrorsNotFound),
		request.NameResponseErrorsRequestTooLarge:  counter(request.NameResponseErrorsRequestTooLarge),
		request.NameResponseErrorsDecode:           counter(request.NameResponseErrorsDecode),
		request.NameResponseErrorsValidate:         counter(request.NameResponseErrorsValidate),
		request.NameResponseErrorsRateLimit:        counter(request.NameResponseErrorsRateLimit),
		request.NameResponseErrorsMethodNotAllowed: counter(request.NameResponseErrorsMethodNotAllowed),
		request.NameResponseErrorsFullQueue:        counter(request.NameResponseErrorsFullQueue),
		request.NameResponseErrorsShuttingDown:     counter(request.NameResponseErrorsShuttingDown),
		request.NameResponseErrorsInternal:         counter(request.NameResponseErrorsInternal),
	}
)

func MonitoringNameToInt(name string) *monitoring.Int {
	if i, ok := mapping[name]; ok {
		return i
	}
	return nil
}
