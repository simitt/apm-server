package main

import (
	"fmt"
	"log"
	"testing"

	"github.com/pkg/errors"
	"go.elastic.co/apm"
	"go.elastic.co/apm/transport"
)

func newTracer(tb testing.TB) *apm.Tracer {
	httpTransport, err := transport.NewHTTPTransport()
	if err != nil {
		tb.Fatal(err)
	}
	tracer, err := apm.NewTracerOptions(apm.TracerOptions{
		Transport: httpTransport,
	})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(tracer.Close)
	return tracer
}

func benchmark100Transactions(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		tracer := newTracer(b)
		for pb.Next() {
			for i := 0; i < 100; i++ {
				withTransaction(tracer)
			}
			tracer.Flush(nil)
		}
		stats := tracer.Stats()
		if n := stats.Errors.SendStream; n > 0 {
			b.Errorf("expected 0 transport errors, got %d", n)
			log.Println("error")
		}
	})
}

func benchmark100TransactionsWithSpans(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		tracer := newTracer(b)
		for pb.Next() {
			for i := 0; i < 100; i++ {
				withSpans(tracer, 45, false)
			}
			tracer.Flush(nil)
		}
		stats := tracer.Stats()
		if n := stats.Errors.SendStream; n > 0 {
			b.Errorf("expected 0 transport errors, got %d", n)
		}
	})
}

func benchmark100TransactionsWithSpansWithStacktraces(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		tracer := newTracer(b)
		for pb.Next() {
			for i := 0; i < 100; i++ {
				withSpans(tracer, 45, true)
			}
			tracer.Flush(nil)
		}
		stats := tracer.Stats()
		if n := stats.Errors.SendStream; n > 0 {
			b.Errorf("expected 0 transport errors, got %d", n)
		}
	})
}

func benchmark100_10Errors(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		tracer := newTracer(b)
		for pb.Next() {
			for i := 0; i < 100; i++ {
				withErrors(tracer, 10)
			}
			tracer.Flush(nil)
		}
		stats := tracer.Stats()
		if n := stats.Errors.SendStream; n > 0 {
			b.Errorf("expected 0 transport errors, got %d", n)
		}
	})
}

func benchmark100_7Errors(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		tracer := newTracer(b)
		for pb.Next() {
			for i := 0; i < 100; i++ {
				withErrors(tracer, 7)
			}
			tracer.Flush(nil)
		}
		stats := tracer.Stats()
		if n := stats.Errors.SendStream; n > 0 {
			b.Errorf("expected 0 transport errors, got %d", n)
		}
	})
}

func benchmark100_5Errors(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		tracer := newTracer(b)
		for pb.Next() {
			for i := 0; i < 100; i++ {
				withErrors(tracer, 5)
			}
			tracer.Flush(nil)
		}
		stats := tracer.Stats()
		if n := stats.Errors.SendStream; n > 0 {
			b.Errorf("expected 0 transport errors, got %d", n)
		}
	})
}

func withTransaction(tracer *apm.Tracer) {
	tx := tracer.StartTransaction("unsampled-transaction", "request")
	defer tx.End()
	tx.Result = "HTTP 2xx"
}

func withSpans(tracer *apm.Tracer, n int, withStacktraces bool) {
	tx := tracer.StartTransaction("with_spans", "request")
	defer tx.End()
	var parentSpan *apm.Span
	for i := 0; i < n; i++ {
		span := tx.StartSpan(fmt.Sprintf("SELECT FROM foo %d", i), "db.mysql.query", parentSpan)
		if withStacktraces {
			span.SetStacktrace(0)
		}
		defer span.End()
	}
	tx.Result = "HTTP 2xx"
}

func withErrors(tracer *apm.Tracer, n int) {
	tx := tracer.StartTransaction("unsampled-transaction", "request")
	defer tx.End()
	err := errors.New("--- initial error")
	for i := 0; i < n; i++ {
		err = errors.Wrap(err, "wrapping ")
	}
	e := tracer.NewError(err)
	e.SetStacktrace(0)
	e.SetTransaction(tx)
	tx.Result = "HTTP 4xx"
	e.Send()
}
