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

func benchmark100_5_5_Spans(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		tracer := newTracer(b)
		for pb.Next() {
			for i := 0; i < 100; i++ {
				withSpans(tracer, 5, 5)
			}
			tracer.Flush(nil)
		}
		stats := tracer.Stats()
		if n := stats.Errors.SendStream; n > 0 {
			b.Errorf("expected 0 transport errors, got %d", n)
		}
	})
}

func benchmark100_15_15_Spans(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		tracer := newTracer(b)
		for pb.Next() {
			for i := 0; i < 100; i++ {
				withSpans(tracer, 15, 15)
			}
			tracer.Flush(nil)
		}
		stats := tracer.Stats()
		if n := stats.Errors.SendStream; n > 0 {
			b.Errorf("expected 0 transport errors, got %d", n)
		}
	})
}

func benchmark100_30_30_Spans(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		tracer := newTracer(b)
		for pb.Next() {
			for i := 0; i < 100; i++ {
				withSpans(tracer, 30, 30)
			}
			tracer.Flush(nil)
		}
		stats := tracer.Stats()
		if n := stats.Errors.SendStream; n > 0 {
			b.Errorf("expected 0 transport errors, got %d", n)
		}
	})
}

func benchmark100_5_5_Errors(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		tracer := newTracer(b)
		for pb.Next() {
			for i := 0; i < 100; i++ {
				withErrors(tracer, 5, 5)
			}
			tracer.Flush(nil)
		}
		stats := tracer.Stats()
		if n := stats.Errors.SendStream; n > 0 {
			b.Errorf("expected 0 transport errors, got %d", n)
		}
	})
}

func benchmark100_10_10_Errors(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		tracer := newTracer(b)
		for pb.Next() {
			for i := 0; i < 100; i++ {
				withErrors(tracer, 10, 10)
			}
			tracer.Flush(nil)
		}
		stats := tracer.Stats()
		if n := stats.Errors.SendStream; n > 0 {
			b.Errorf("expected 0 transport errors, got %d", n)
		}
	})
}

func benchmark100_15_15_Errors(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		tracer := newTracer(b)
		for pb.Next() {
			for i := 0; i < 100; i++ {
				withErrors(tracer, 15, 15)
			}
			tracer.Flush(nil)
		}
		stats := tracer.Stats()
		if n := stats.Errors.SendStream; n > 0 {
			b.Errorf("expected 0 transport errors, got %d", n)
		}
	})
}

func benchmark100_1_30_Errors(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		tracer := newTracer(b)
		for pb.Next() {
			for i := 0; i < 100; i++ {
				withErrors(tracer, 1, 30)
			}
			tracer.Flush(nil)
		}
		stats := tracer.Stats()
		if n := stats.Errors.SendStream; n > 0 {
			b.Errorf("expected 0 transport errors, got %d", n)
		}
	})
}

func benchmark100_10_30_Errors(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		tracer := newTracer(b)
		for pb.Next() {
			for i := 0; i < 100; i++ {
				withErrors(tracer, 10, 30)
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

func withSpans(tracer *apm.Tracer, spans int, stacktraces int) {
	tx := tracer.StartTransaction(fmt.Sprintf("with-spans-%d-stacktraces-%d", spans, stacktraces), "request")
	defer tx.End()
	var parentSpan *apm.Span
	for i := 0; i < spans; i++ {
		span := tx.StartSpan(fmt.Sprintf("SELECT FROM foo %d", i), "db.mysql.query", parentSpan)
		if stacktraces > 0 {
			spanWithStacktrace(span, 0, stacktraces)
		}
		defer span.End()
	}
	tx.Result = "HTTP 2xx"
}

func spanWithStacktrace(span *apm.Span, level int, N int) {
	if level < N {
		spanWithStacktrace(span, level+1, N)
	} else {
		span.SetStacktrace(0)
	}
}

func withErrors(tracer *apm.Tracer, errors int, stacktraces int) {
	tx := tracer.StartTransaction(fmt.Sprintf("with-errors-%d-stacktraces-%d", errors, stacktraces), "request")
	defer tx.End()
	for i := 0; i < errors; i++ {
		e := errorWithStacktraces(tracer, stacktraces)
		e.SetTransaction(tx)
		e.Send()
	}
	tx.Result = "HTTP 4xx"
}

func errorWithStacktraces(tracer *apm.Tracer, N int) *apm.Error {
	err := errors.New("my error")
	for i := 0; i < N; i++ {
		err = errors.Wrap(err, "wrapping ")
	}
	e := tracer.NewError(err)
	e.SetStacktrace(0)
	return e
}
