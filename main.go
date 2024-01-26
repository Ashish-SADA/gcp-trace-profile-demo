package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/profiler"
	texporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"go.opentelemetry.io/otel/trace"
)

type Application struct {
	ctx       context.Context
	mux       *http.ServeMux
	logger    *logging.Logger
	projectID string
	exporter  *texporter.Exporter
	resource  *resource.Resource
	tp        *sdktrace.TracerProvider
}

func main() {

	cfg := profiler.Config{
		Service:        "pingpong",
		ServiceVersion: "1.0.0",
	}

	// Profiler initialization, best done as early as possible.
	if err := profiler.Start(cfg); err != nil {
		log.Fatal(err)
	}

	var app = Application{
		mux:       http.NewServeMux(),
		ctx:       context.Background(),
		projectID: os.Getenv("GOOGLE_CLOUD_PROJECT"),
	}

	client, _ := logging.NewClient(app.ctx, app.projectID)
	defer client.Close()

	logName := "pingpong"

	app.logger = client.Logger(logName)
	defer app.logger.Flush()

	var err error
	app.exporter, err = texporter.New(texporter.WithProjectID(app.projectID))
	if err != nil {
		app.logger.Log(logging.Entry{
			Severity: logging.Error,
			Payload:  "textporter new",
		})
	}

	// Identify your application using resource detection
	app.resource, err = resource.New(app.ctx,
		// Use the GCP resource detector to detect information about the GCP platform
		resource.WithDetectors(gcp.NewDetector()),
		// Keep the default detectors
		resource.WithTelemetrySDK(),
		// Add your own custom attributes to identify your application
		resource.WithAttributes(
			semconv.ServiceNameKey.String("pingpong-service"),
		),
	)
	if err != nil {
		app.logger.Log(logging.Entry{
			Severity: logging.Error,
			Payload:  "Error creating resource",
		})
	}
	// Create trace provider with the exporter.
	//
	// By default it uses AlwaysSample() which samples all traces.
	// In a production environment or high QPS setup please use
	// probabilistic sampling.
	// Example:
	//   tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.0001)), ...)
	app.tp = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(app.exporter),
		sdktrace.WithResource(app.resource),
	)
	defer app.tp.Shutdown(app.ctx) // flushes any pending spans, and closes connections.
	otel.SetTracerProvider(app.tp)

	app.mux.HandleFunc("/ping", app.pingFunc)
	app.mux.HandleFunc("/pong", app.pongFunc)

	log.Fatal(http.ListenAndServe(":8080", app.mux))
}

func RandBool() bool {
	return rand.Intn(2) == 1
}

func (a *Application) createTrace(t string) string {
	return fmt.Sprintf("projects/%s/traces/%s", a.projectID, t)
}

func (a *Application) pingFunc(w http.ResponseWriter, r *http.Request) {
	tracer := otel.GetTracerProvider().Tracer("sada.com/jorge-app")
	var span trace.Span

	reqCtx := r.Context()

	a.ctx, span = tracer.Start(reqCtx, "ping")
	defer span.End()

	if RandBool() {
		a.logger.Log(logging.Entry{
			Trace:    a.createTrace(span.SpanContext().TraceID().String()),
			SpanID:   span.SpanContext().SpanID().String(),
			Payload:  "We zonged when we should've ponged",
			Severity: logging.Error,
		})
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("zong"))
	} else {
		a.logger.Log(logging.Entry{
			Trace:    a.createTrace(span.SpanContext().TraceID().String()),
			SpanID:   span.SpanContext().SpanID().String(),
			Payload:  "We ponged",
			Severity: logging.Info,
		})
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pong"))
	}

}
func (a *Application) pongFunc(w http.ResponseWriter, r *http.Request) {
	tracer := otel.GetTracerProvider().Tracer("sada.com/jorge-app")
	var span trace.Span

	a.ctx, span = tracer.Start(r.Context(), "pong")
	defer span.End()

	if RandBool() {
		a.logger.Log(logging.Entry{
			Trace:    a.createTrace(span.SpanContext().TraceID().String()),
			SpanID:   span.SpanContext().SpanID().String(),
			Payload:  "Looks like we zinged when we should ping",
			Severity: logging.Error,
		})
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("zing"))
	} else {
		a.logger.Log(logging.Entry{
			Trace:    a.createTrace(span.SpanContext().TraceID().String()),
			SpanID:   span.SpanContext().SpanID().String(),
			Payload:  "We pinged",
			Severity: logging.Info,
		})
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ping"))
	}

}
