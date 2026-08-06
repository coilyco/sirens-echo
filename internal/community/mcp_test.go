package community

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestMCPProviderAllowsEmptyRoster(t *testing.T) {
	t.Parallel()
	session, err := (MCPProvider{}).Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if tools := session.Tools(); len(tools) != 0 {
		t.Fatalf("tools = %#v", tools)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestMCPProviderClosesCleanupResponseSpan(t *testing.T) {
	mcpServer := mcp.NewServer(
		&mcp.Implementation{Name: "eco-test", Version: "1"},
		nil,
	)
	streamable := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return mcpServer },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	)
	var deleteRequests atomic.Int32
	var headersMu sync.Mutex
	traceParents := make(map[string][]string)
	httpServer := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		headersMu.Lock()
		traceParents[request.Method] = append(
			traceParents[request.Method],
			request.Header.Get("traceparent"),
		)
		headersMu.Unlock()
		if request.Method == http.MethodDelete {
			deleteRequests.Add(1)
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		streamable.ServeHTTP(writer, request)
	}))
	t.Cleanup(httpServer.Close)

	recorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(recorder),
	)
	t.Cleanup(func() {
		_ = traceProvider.Shutdown(context.Background())
	})
	httpClient := &http.Client{
		Timeout: time.Second,
		Transport: otelhttp.NewTransport(
			http.DefaultTransport,
			otelhttp.WithTracerProvider(traceProvider),
			otelhttp.WithPropagators(propagation.TraceContext{}),
		),
	}
	ctx, parent := traceProvider.Tracer("sirens-echo-test").Start(
		context.Background(),
		"community.turn",
	)
	parentContext := parent.SpanContext()

	session, err := (MCPProvider{
		Servers:    []MCPServerDefinition{{Name: "eco", URL: httpServer.URL}},
		HTTPClient: httpClient,
	}).Open(ctx)
	if err != nil {
		parent.End()
		t.Fatalf("Open: %v", err)
	}
	if err := session.Close(); err != nil {
		parent.End()
		t.Fatalf("Close: %v", err)
	}
	parent.End()

	if got := deleteRequests.Load(); got != 1 {
		t.Fatalf("DELETE requests = %d, want 1", got)
	}
	endedMethods := make(map[string]int)
	for _, span := range recorder.Ended() {
		if span.SpanKind() != trace.SpanKindClient {
			continue
		}
		method := spanStringAttribute(span.Attributes(), "http.request.method")
		if method != http.MethodPost && method != http.MethodDelete {
			continue
		}
		endedMethods[method]++
		if span.SpanContext().TraceID() != parentContext.TraceID() ||
			span.Parent().SpanID() != parentContext.SpanID() {
			t.Errorf("%s client span is outside the turn trace", method)
		}
	}
	if endedMethods[http.MethodPost] == 0 {
		t.Fatal("ordinary MCP POST client spans did not end")
	}
	if endedMethods[http.MethodDelete] != 1 {
		t.Fatalf(
			"ended MCP DELETE client spans = %d, want 1",
			endedMethods[http.MethodDelete],
		)
	}

	headersMu.Lock()
	defer headersMu.Unlock()
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		if len(traceParents[method]) == 0 {
			t.Fatalf("%s traceparent headers = %#v", method, traceParents[method])
		}
		for _, traceParent := range traceParents[method] {
			if !strings.Contains(traceParent, parentContext.TraceID().String()) {
				t.Errorf("%s traceparent = %q", method, traceParent)
			}
		}
	}
}

func spanStringAttribute(attributes []attribute.KeyValue, key attribute.Key) string {
	for _, item := range attributes {
		if item.Key == key {
			return item.Value.AsString()
		}
	}
	return ""
}
