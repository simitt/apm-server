package api

import (
	"net/http"
	"sync"

	"github.com/elastic/apm-server/beater/request"
)

type contextPool struct {
	p sync.Pool
}

func newContextPool() *contextPool {
	pool := contextPool{}
	pool.p.New = func() interface{} {
		return &request.Context{}
	}
	return &pool
}

func (pool *contextPool) handler(h request.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := pool.p.Get().(*request.Context)
		defer pool.p.Put(c)
		c.Reset(w, r)

		h(c)
	})
}
