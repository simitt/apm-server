package middleware

import (
	"errors"

	"github.com/elastic/apm-server/beater/request"
)

func KillSwitchHandler(killSwitch bool) Middleware {
	return func(h request.Handler) request.Handler {
		return func(c *request.Context) {
			if killSwitch {
				h(c)
			} else {
				request.ForbiddenResult(errors.New("endpoint is disabled")).WriteTo(c)
			}
		}
	}
}
