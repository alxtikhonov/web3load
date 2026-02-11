// Package telemetry provides optional OpenTelemetry tracing for a run: one
// span per executed action step, so a slow or failing transaction can be
// traced end-to-end alongside whatever else the operator's collector
// ingests. Setup is opt-in — until it's called, otel.Tracer falls back to
// the library's built-in no-op implementation, so instrumentation elsewhere
// in the codebase can call StartStep/RecordOutcome unconditionally at
// effectively zero cost when tracing isn't configured.
package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("web3load")

// Setup configures the global TracerProvider to batch-export spans to an
// OTLP/HTTP collector at endpoint (host:port, no scheme — e.g.
// "localhost:4318"; TLS is not used, matching a local/sidecar collector).
// Call the returned shutdown func (e.g. via defer) to flush and close on
// exit; skipping Setup entirely is the normal, zero-cost path when tracing
// isn't wanted.
func Setup(ctx context.Context, endpoint, serviceName string) (shutdown func(context.Context) error, err error) {
	exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(endpoint), otlptracehttp.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("telemetry: create OTLP exporter for %s: %w", endpoint, err)
	}
	res, err := resource.New(ctx, resource.WithAttributes(attribute.String("service.name", serviceName)))
	if err != nil {
		return nil, fmt.Errorf("telemetry: create resource: %w", err)
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp), sdktrace.WithResource(res))
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// StartStep starts a span covering one action step execution. Safe to call
// whether or not Setup was ever invoked.
func StartStep(ctx context.Context, actionName, walletAddr string) (context.Context, trace.Span) {
	return tracer.Start(ctx, "web3load.action."+actionName, trace.WithAttributes(
		attribute.String("web3load.action", actionName),
		attribute.String("web3load.wallet", walletAddr),
	))
}

// RecordOutcome annotates a step's span with its result and ends it.
func RecordOutcome(span trace.Span, txHash string, gasUsed uint64, err error) {
	if txHash != "" {
		span.SetAttributes(attribute.String("web3load.tx_hash", txHash))
	}
	if gasUsed > 0 {
		span.SetAttributes(attribute.Int64("web3load.gas_used", int64(gasUsed)))
	}
	if err != nil {
		span.RecordError(err)
	}
	span.End()
}
