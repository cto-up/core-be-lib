package observability

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// Agent traces, as opposed to the product trace in tracing.go (hub#88).
//
// Sentry follows a REQUEST for the team. These follow a piece of work that has
// already happened — an agent run whose steps are persisted — to wherever its
// owner analyses it: their own Langfuse, Braintrust or Arize. So nothing here is
// a live tracer. A finished record is turned into spans and sent once, which is
// what lets a span keep the id of the row it came from, and what means a
// destination per tenant needs no provider kept open per tenant.

// OTLPTarget is one destination, described the way OTEL_EXPORTER_OTLP_ENDPOINT
// and OTEL_EXPORTER_OTLP_HEADERS describe one.
type OTLPTarget struct {
	// Endpoint is the base URL; /v1/traces is appended unless it is already
	// there, e.g. https://cloud.langfuse.com/api/public/otel.
	Endpoint string
	Headers  map[string]string
}

// SpanRecord is one span whose outcome is already known.
type SpanRecord struct {
	TraceID trace.TraceID
	SpanID  trace.SpanID
	// ParentID is the zero SpanID for a root.
	ParentID   trace.SpanID
	Name       string
	Kind       trace.SpanKind
	Start      time.Time
	End        time.Time
	Attributes []attribute.KeyValue
	// Err non-empty marks the span failed, with this as its description.
	Err string
}

// TraceIDFrom keys a trace to a 16-byte id, such as a UUID, so a trace found
// in someone else's tool leads straight back to the record it came from.
func TraceIDFrom(id [16]byte) trace.TraceID { return trace.TraceID(id) }

// SpanIDFrom derives a span id from a 16-byte id. The LAST eight bytes: a
// time-ordered UUID starts with its timestamp, and two steps recorded in the
// same millisecond would otherwise share a span id.
func SpanIDFrom(id [16]byte) trace.SpanID {
	var s trace.SpanID
	copy(s[:], id[8:])
	return s
}

// ExportSpans sends spans to target in one request and returns once the
// destination has answered. The exporter lives for this call only.
//
// resource describes who produced the spans (service.name, service.version).
func ExportSpans(ctx context.Context, target OTLPTarget, res []attribute.KeyValue, spans []SpanRecord) error {
	if strings.TrimSpace(target.Endpoint) == "" {
		return errors.New("otlp: no endpoint")
	}
	if len(spans) == 0 {
		return nil
	}
	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(tracesURL(target.Endpoint)),
		otlptracehttp.WithHeaders(target.Headers),
	)
	if err != nil {
		return fmt.Errorf("otlp: exporter: %w", err)
	}
	defer func() { _ = exp.Shutdown(context.WithoutCancel(ctx)) }()

	r := resource.NewSchemaless(res...)
	stubs := make(tracetest.SpanStubs, 0, len(spans))
	for _, s := range spans {
		stub := tracetest.SpanStub{
			Name: s.Name,
			SpanContext: trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    s.TraceID,
				SpanID:     s.SpanID,
				TraceFlags: trace.FlagsSampled,
			}),
			SpanKind:             s.Kind,
			StartTime:            s.Start,
			EndTime:              s.End,
			Attributes:           s.Attributes,
			Resource:             r,
			InstrumentationScope: instrumentation.Scope{Name: "ctoup.com/coreapp/observability"},
		}
		if s.ParentID.IsValid() {
			stub.Parent = trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    s.TraceID,
				SpanID:     s.ParentID,
				TraceFlags: trace.FlagsSampled,
			})
		}
		if s.Err != "" {
			stub.Status = sdktrace.Status{Code: codes.Error, Description: s.Err}
		}
		stubs = append(stubs, stub)
	}
	if err := exp.ExportSpans(ctx, stubs.Snapshots()); err != nil {
		return fmt.Errorf("otlp: export: %w", err)
	}
	return nil
}

// ParseOTLPHeaders reads the OTEL_EXPORTER_OTLP_HEADERS format:
// "Authorization=Basic%20abc,x-team=ai". Values are percent-decoded; a pair
// without a key is dropped rather than failing the whole list.
func ParseOTLPHeaders(s string) map[string]string {
	out := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(pair, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			continue
		}
		v = strings.TrimSpace(v)
		if decoded, err := url.PathUnescape(v); err == nil {
			v = decoded
		}
		out[k] = v
	}
	return out
}

func tracesURL(endpoint string) string {
	e := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if strings.HasSuffix(e, "/v1/traces") {
		return e
	}
	return e + "/v1/traces"
}
