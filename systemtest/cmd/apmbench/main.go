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

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"testing"

	"go.elastic.co/apm"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/elastic/apm-server/systemtest/benchtest"
)

func BenchmarkOTLPTraces(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		exporter := benchtest.NewOTLPExporter(b)
		tracerProvider := sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
			sdktrace.WithBatcher(exporter, sdktrace.WithBlocking()),
		)
		tracer := tracerProvider.Tracer("tracer")
		for pb.Next() {
			_, span := tracer.Start(context.Background(), "name")
			span.End()
		}
		if err := tracerProvider.ForceFlush(context.Background()); err != nil {
			b.Fatal(err)
		}
		if err := exporter.Shutdown(context.Background()); err != nil {
			b.Fatal(err)
		}
	})
}

func Benchmark1000Transactions(b *testing.B) {
	runBenchmark(b, 0, 0, withSpans)
}

func Benchmark1000_50_100_Spans(b *testing.B) {
	runBenchmark(b, 50, 100, withSpans)
}
func Benchmark1000_50_75_Spans(b *testing.B) {
	runBenchmark(b, 50, 75, withSpans)
}

func Benchmark1000_50_50_Spans(b *testing.B) {
	runBenchmark(b, 50, 50, withSpans)
}

func Benchmark1000_30_30_Spans(b *testing.B) {
	runBenchmark(b, 30, 30, withSpans)
}

func Benchmark1000_15_15_Spans(b *testing.B) {
	runBenchmark(b, 15, 15, withSpans)
}

func Benchmark1000_5_5_Spans(b *testing.B) {
	runBenchmark(b, 5, 5, withSpans)
}
func Benchmark1000_50_100_Errors(b *testing.B) {
	runBenchmark(b, 50, 100, withErrors)
}

func Benchmark1000_50_75_Errors(b *testing.B) {
	runBenchmark(b, 50, 75, withErrors)
}

func Benchmark1000_50_50_Errors(b *testing.B) {
	runBenchmark(b, 50, 50, withErrors)
}

func Benchmark1000_30_30_Errors(b *testing.B) {
	runBenchmark(b, 30, 30, withErrors)
}

func Benchmark1000_15_15_Errors(b *testing.B) {
	runBenchmark(b, 15, 15, withErrors)
}

func Benchmark1000_5_5_Errors(b *testing.B) {
	runBenchmark(b, 5, 5, withErrors)
}

func runBenchmark(b *testing.B, n int, frames int, fn func(*apm.Tracer, int, int)) {
	b.RunParallel(func(pb *testing.PB) {
		tracer := benchtest.NewTracer(b)
		for pb.Next() {
			for i := 0; i < 100; i++ {
				fn(tracer, n, frames)
			}
			// TODO(axw) implement a transport that enables streaming
			// events in a way that we can block when the queue is full,
			// without flushing. Alternatively, make this an option in
			// TracerOptions?
			tracer.Flush(nil)
		}
		stats := tracer.Stats()
		if n := stats.Errors.SendStream; n > 0 {
			fmt.Println(fmt.Sprintf("expected 0 transport errors, got %d", n))
			// b.Errorf("expected 0 transport errors, got %d", n)
		}
	})

}

func withSpans(tracer *apm.Tracer, n int, stacktraces int) {
	// set the maximum number of stacktraces
	tracer.SetStackTraceLimit(stacktraces)
	// create a transaction with n spans
	tx := tracer.StartTransaction(fmt.Sprintf("with-spans-%d-stacktraces-%d", n, stacktraces), "request")
	defer tx.End()
	var parentSpan *apm.Span
	for i := 0; i < n; i++ {
		span := tx.StartSpan(fmt.Sprintf("SELECT FROM foo %d", i), "db.mysql.query", parentSpan)
		if stacktraces > 0 {
			spanWithStacktrace(span, stacktraces, 0)
		}
		defer span.End()
	}
	tx.Result = "HTTP 2xx"
}

func spanWithStacktrace(span *apm.Span, n int, current int) {
	if current < n {
		spanWithStacktrace(span, n, current+1)
	} else {
		span.SetStacktrace(0)
	}
}

func withErrors(tracer *apm.Tracer, n int, stacktraces int) {
	// set the maximum number of stacktraces
	tracer.SetStackTraceLimit(stacktraces)
	// create a transaction with n errors
	tx := tracer.StartTransaction(fmt.Sprintf("with-errors-%d-stacktraces-%d", n, stacktraces), "request")
	defer tx.End()
	for i := 0; i < n; i++ {
		e := tracer.NewError(errors.New("boom"))
		if stacktraces > 0 {
			errorWithStacktrace(e, stacktraces, 0)
		}
		e.SetTransaction(tx)
		e.Send()
	}
	tx.Result = "HTTP 4xx"
}

func errorWithStacktrace(e *apm.Error, n int, current int) {
	if current < n {
		errorWithStacktrace(e, n, current+1)
	} else {
		e.SetStacktrace(0)
	}
}

func main() {
	scenario := "high_mem"
	var benchmarks []benchtest.BenchmarkFunc
	switch scenario {
	case "high_mem":
		benchmarks = []benchtest.BenchmarkFunc{
			Benchmark1000_50_100_Errors,
			Benchmark1000_50_75_Errors,
			Benchmark1000_50_50_Errors,
			Benchmark1000_50_100_Spans,
			Benchmark1000_50_75_Spans,
			Benchmark1000_50_50_Spans,
		}
	case "documented_load":
		benchmarks = []benchtest.BenchmarkFunc{
			Benchmark1000_30_30_Errors,
			Benchmark1000_15_15_Errors,
			Benchmark1000_5_5_Errors,
			Benchmark1000_30_30_Spans,
			Benchmark1000_15_15_Spans,
			Benchmark1000_5_5_Spans,
			Benchmark1000Transactions,
		}
	default:
		benchmarks = []benchtest.BenchmarkFunc{
			Benchmark1000_50_50_Errors,
			Benchmark1000_30_30_Errors,
			Benchmark1000_15_15_Errors,
			Benchmark1000_5_5_Errors,
			Benchmark1000_50_50_Spans,
			Benchmark1000_30_30_Spans,
			Benchmark1000_15_15_Spans,
			Benchmark1000_5_5_Spans,
			Benchmark1000Transactions,
			BenchmarkOTLPTraces,
		}
	}
	if err := benchtest.Run(benchmarks...); err != nil {
		log.Fatal(err)
	}
}
