package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func Bootstrap(ctx context.Context) (func(), error) {
	// Set up resource.
	res, err := resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceName("gh"),
			semconv.ServiceVersion("0.0.0"),
		))
	if err != nil {
		return nil, err
	}

	// Set up propagator.
	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(prop)

	conn, err := getConnection(ctx)
	if err != nil {
		return nil, err
	}

	te, err := getOTLPTraceExporter(ctx, conn)
	if err != nil {
		return nil, err
	}

	traceProvider := trace.NewTracerProvider(
		trace.WithSyncer(te),
		trace.WithResource(res),
	)
	otel.SetTracerProvider(traceProvider)

	me, err := getOTLPMetricExporter(ctx, conn)
	if err != nil {
		return nil, err
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(me, metric.WithInterval(10*time.Millisecond))),
		metric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)

	return func() {
		if err := meterProvider.ForceFlush(ctx); err != nil {
			fmt.Printf("error forcing flush: %v\n", err)
		}
		if err := meterProvider.Shutdown(ctx); err != nil {
			fmt.Printf("error shutting down meter provider: %v\n", err)
		}
		if err := traceProvider.Shutdown(ctx); err != nil {
			fmt.Printf("error shutting down trace provider: %v\n", err)
		}

	}, nil
}

func getConnection(ctx context.Context) (*grpc.ClientConn, error) {
	conn, err := grpc.DialContext(ctx, "localhost:4317",
		// Note the use of insecure transport here. TLS is recommended in production.
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection to collector: %w", err)
	}

	return conn, nil
}

func getOTLPTraceExporter(ctx context.Context, conn *grpc.ClientConn) (*otlptrace.Exporter, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	// Set up a trace exporter
	return otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
}

func getOTLPMetricExporter(ctx context.Context, conn *grpc.ClientConn) (*otlpmetricgrpc.Exporter, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	// Set up a trace exporter
	return otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(conn))
}
