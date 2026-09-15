package observability

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// collector is a stand-in OTLP/HTTP receiver that keeps what it was sent.
type collector struct {
	path    string
	headers http.Header
	req     *coltracepb.ExportTraceServiceRequest
}

func (c *collector) serve(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		c.path, c.headers = r.URL.Path, r.Header.Clone()
		c.req = &coltracepb.ExportTraceServiceRequest{}
		require.NoError(t, proto.Unmarshal(body, c.req))
		out, _ := proto.Marshal(&coltracepb.ExportTraceServiceResponse{})
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(out)
	}))
}

func TestATraceArrivesWholeWithItsTreeAndIdentity(t *testing.T) {
	c := &collector{}
	srv := c.serve(t)
	defer srv.Close()

	run := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	step := [16]byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	start := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	spans := []SpanRecord{
		{TraceID: TraceIDFrom(run), SpanID: SpanIDFrom(run), Name: "invoke_agent Ana",
			Start: start, End: start.Add(3 * time.Second)},
		{TraceID: TraceIDFrom(run), SpanID: SpanIDFrom(step), ParentID: SpanIDFrom(run), Name: "chat gpt-4o",
			Start: start.Add(time.Second), End: start.Add(2 * time.Second),
			Attributes: []attribute.KeyValue{attribute.Int("gen_ai.usage.input_tokens", 12)},
			Err:        "provider said no"},
	}

	err := ExportSpans(context.Background(),
		OTLPTarget{Endpoint: srv.URL + "/api/public/otel", Headers: map[string]string{"Authorization": "Basic abc"}},
		[]attribute.KeyValue{attribute.String("service.name", "aiemployee")},
		spans)

	require.NoError(t, err)
	require.Equal(t, "/api/public/otel/v1/traces", c.path)
	require.Equal(t, "Basic abc", c.headers.Get("Authorization"))

	require.Len(t, c.req.ResourceSpans, 1)
	rs := c.req.ResourceSpans[0]
	require.Equal(t, "service.name", rs.Resource.Attributes[0].Key)
	got := rs.ScopeSpans[0].Spans
	require.Len(t, got, 2)

	root, child := got[0], got[1]
	require.Equal(t, run[:], root.TraceId, "the trace id is the record's id, so it can be looked up back home")
	require.Empty(t, root.ParentSpanId)
	require.Equal(t, root.SpanId, child.ParentSpanId)
	require.Equal(t, step[8:], child.SpanId)
	require.Equal(t, uint64(start.Add(time.Second).UnixNano()), child.StartTimeUnixNano)
	require.Equal(t, tracepb.Status_STATUS_CODE_ERROR, child.Status.Code)
	require.Equal(t, "provider said no", child.Status.Message)
	require.Equal(t, int64(12), child.Attributes[0].Value.GetIntValue())
}

func TestAnUnreachableDestinationIsAnErrorNotAHang(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	id := [16]byte{1}
	err := ExportSpans(ctx, OTLPTarget{Endpoint: url}, nil,
		[]SpanRecord{{TraceID: TraceIDFrom(id), SpanID: SpanIDFrom(id), Name: "x", Start: time.Now(), End: time.Now()}})

	require.Error(t, err)
}

func TestNoEndpointSendsNothing(t *testing.T) {
	require.Error(t, ExportSpans(context.Background(), OTLPTarget{}, nil, []SpanRecord{{Name: "x"}}))
}

func TestHeadersReadTheOTelEnvironmentFormat(t *testing.T) {
	require.Equal(t,
		map[string]string{"Authorization": "Basic YWI6Y2Q=", "x-team": "ai"},
		ParseOTLPHeaders(" Authorization=Basic%20YWI6Y2Q= , x-team=ai,=orphan,novalue"))
}

func TestTheTracesPathIsAppendedOnce(t *testing.T) {
	require.Equal(t, "https://h/otel/v1/traces", tracesURL("https://h/otel/"))
	require.Equal(t, "https://h/v1/traces", tracesURL("https://h/v1/traces"))
}
