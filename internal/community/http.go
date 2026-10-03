package community

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	httpTurnPath = "/v1/turn"
	healthzPath  = "/healthz"
)

// Shared so both write endpoints refuse in the same words, and derived from the
// cap so the number in the message cannot drift away from the one enforced.
var oversizeBodyMessage = fmt.Sprintf("request body exceeds the %d byte limit", maxHTTPBody)

type httpTurnRequest struct {
	RequestID string            `json:"request_id"`
	Author    string            `json:"author"`
	Content   string            `json:"content"`
	History   []TranscriptEntry `json:"history"`
	// Prompt names a server prompt the caller selected. Naming one is what makes
	// the selection user-controlled, which is how the spec defines prompts.
	Prompt *httpPromptRequest `json:"prompt,omitempty"`
}

type httpPromptRequest struct {
	Server    string            `json:"server"`
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments,omitempty"`
}

type httpTurnResponse struct {
	Reply    string `json:"reply,omitempty"`
	Reaction string `json:"reaction,omitempty"`
	Error    string `json:"error,omitempty"`
}

// HTTPHandler exposes the same turn path as a summoned Discord message after
// Discord-specific gates pass. It is intended for private or loopback tests.
func (a *Agent) HTTPHandler() http.Handler {
	telemetry := telemetryOrNoop(a.telemetry)
	a.telemetry = telemetry
	a.ensureRuntimeDefaults()
	mux := http.NewServeMux()
	mux.HandleFunc(healthzPath, a.handleHealthz)
	mux.HandleFunc(readyzPath, a.handleReadyz)
	mux.Handle(httpTurnPath, instrumentHTTPRoute(telemetry, httpTurnPath, a.handleHTTPTurn))
	// A post is an outbound action, so it sits beside the turn path rather than
	// inside it. Other actions arrive as their own paths with their own allowlists.
	mux.Handle(httpMessagePath, instrumentHTTPRoute(telemetry, httpMessagePath, a.handleHTTPMessage))
	// Submit, poll, and cancel share the turn path's network boundary.
	mux.Handle("/v1/jobs", instrumentHTTPRoute(telemetry, "/v1/jobs", a.handleJobs))
	mux.Handle(jobsPath, instrumentHTTPRoute(telemetry, jobsPath, a.handleJob))
	// Same listener and therefore the same network boundary as the turn path.
	mux.Handle(mcpServerPath, a.mcpHandler())
	return mux
}

func instrumentHTTPRoute(
	telemetry *Telemetry,
	route string,
	handler http.HandlerFunc,
) http.Handler {
	routeHandler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		trace.SpanFromContext(request.Context()).SetAttributes(attribute.String("http.route", route))
		handler(writer, request)
	})
	return otelhttp.NewHandler(
		routeHandler,
		route,
		otelhttp.WithTracerProvider(telemetry.traceProvider),
		otelhttp.WithPropagators(telemetry.propagator),
		otelhttp.WithSpanNameFormatter(func(_ string, request *http.Request) string {
			return request.Method + " " + route
		}),
	)
}

func (a *Agent) handleHealthz(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		a.telemetry.RecordHealth(request.Context(), "healthz", "method_not_allowed")
		writer.Header().Set("Allow", http.MethodGet)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.telemetry.RecordHealth(request.Context(), "healthz", "ready")
	// Always 200, so the age informs a probe and never restarts the process.
	body := map[string]any{"ok": true}
	if age, ok := a.beats.age(time.Now()); ok {
		body["gateway_beat_age_seconds"] = int64(age / time.Second)
	}
	writeJSON(writer, http.StatusOK, body)
}

func (a *Agent) handleReadyz(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		a.telemetry.RecordHealth(request.Context(), "readyz", "method_not_allowed")
		writer.Header().Set("Allow", http.MethodGet)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	started := time.Now()
	outcome := a.checkRouteReadiness(request.Context())
	a.telemetry.RecordHealth(request.Context(), "readyz", string(outcome))
	a.telemetry.RecordReadiness(request.Context(), outcome, time.Since(started))
	if outcome != readinessReady {
		writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ready"})
}

func (a *Agent) handleHTTPTurn(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		a.writeHTTPError(
			writer,
			request,
			http.StatusMethodNotAllowed,
			exceptionHTTPTurnMethodNotAllowed,
			"method not allowed",
		)
		return
	}
	var payload httpTurnRequest
	if !a.decodeHTTPBody(writer, request, a.writeHTTPError, &payload) {
		return
	}
	// A selected prompt is itself the request, so content becomes optional only
	// when the caller named one.
	if strings.TrimSpace(payload.Content) == "" && payload.Prompt == nil {
		a.writeHTTPError(
			writer,
			request,
			http.StatusBadRequest,
			exceptionHTTPTurnContentRequired,
			"content is required",
		)
		return
	}
	if payload.Author == "" {
		payload.Author = "manual test"
	}
	if len([]rune(payload.Author)) > 256 || len([]rune(payload.Content)) > 16000 {
		a.writeHTTPError(
			writer,
			request,
			http.StatusBadRequest,
			exceptionHTTPTurnInputTooLong,
			"author or content is too long",
		)
		return
	}
	if len(payload.History) > a.cfg.Definition.MaxContextMessages {
		a.writeHTTPError(
			writer,
			request,
			http.StatusBadRequest,
			exceptionHTTPTurnHistoryTooLong,
			"history exceeds the configured context limit",
		)
		return
	}
	if payload.RequestID == "" {
		payload.RequestID = fmt.Sprintf("http-%d", time.Now().UnixNano())
	}

	history := assertedHistory(payload.History)
	current := TranscriptEntry{Author: payload.Author, Content: payload.Content}
	if payload.Prompt != nil {
		resolved, err := a.resolvePrompt(request.Context(), payload.Prompt)
		if err != nil {
			// Only a caller-fixable message is echoed back. A transport failure
			// could carry an endpoint, so it stays generic.
			message := "prompt could not be resolved"
			var caller PromptRequestError
			if errors.As(err, &caller) {
				message = caller.Message
			}
			a.writeHTTPError(
				writer,
				request,
				http.StatusBadRequest,
				exceptionHTTPTurnPromptFailed,
				message,
			)
			return
		}
		history, current = seedFromPrompt(history, current, resolved)
	}
	trusted := callerTrusted(request, a.cfg.HTTPTrustToken)
	turn := &httpTurn{
		requestID: payload.RequestID,
		requester: httpPrincipal(request),
		history:   history,
		current:   current,
		trusted:   trusted,
	}
	// Recorded rather than only decided, so an operator can tell a trusted call
	// from an anonymous one. The token itself never leaves the process.
	trace.SpanFromContext(request.Context()).SetAttributes(
		attribute.Bool("http.caller.trusted", trusted),
	)
	// HTTP shares the Discord admission policy, so a scripted client cannot
	// outspend the guilds it shares a deployment with.
	decision := a.limiter.Admit(admissionRequest{
		UserKey:    httpPrincipal(request),
		ContextKey: transportHTTP,
		Queued:     true,
	})
	if decision.Outcome.denied() {
		a.telemetry.RecordAdmission(request.Context(), string(decision.Outcome), transportHTTP)
		if decision.RetryAfter > 0 {
			writer.Header().Set(
				"Retry-After",
				strconv.Itoa(int(math.Ceil(decision.RetryAfter.Seconds()))),
			)
		}
		a.writeHTTPError(
			writer,
			request,
			http.StatusTooManyRequests,
			exceptionHTTPTurnRateLimited,
			"rate limit reached",
		)
		return
	}
	defer a.limiter.Release()
	a.telemetry.RecordAdmission(request.Context(), string(admissionAccepted), transportHTTP)

	if err := a.runSerialized(request.Context(), turn); err != nil {
		writeJSON(writer, http.StatusBadGateway, httpTurnResponse{
			Reply: turn.reply,
			Error: "turn failed",
		})
		return
	}
	writeJSON(writer, http.StatusOK, httpTurnResponse{Reply: turn.reply, Reaction: turn.reaction})
}

// httpRefusal is the shape of writeHTTPError and writeMessageHTTPError, passed
// in so one decoder serves routes that log their refusals under different events.
type httpRefusal func(
	http.ResponseWriter, *http.Request, int, exceptionCode, string,
)

// decodeHTTPBody reads one JSON object into destination and refuses, in the
// contract's words, when it cannot. False means the response is already written.
func (a *Agent) decodeHTTPBody(
	writer http.ResponseWriter,
	request *http.Request,
	refuse httpRefusal,
	destination any,
) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, int64(maxHTTPBody))
	decoder := json.NewDecoder(request.Body)
	// A field the contract does not define took no effect, and a 200 tells the
	// caller it did. See docs/sirens-echo-http.md.
	decoder.DisallowUnknownFields()
	err := decoder.Decode(destination)
	if err == nil {
		return true
	}
	if oversizeBody(err) {
		refuse(writer, request, http.StatusBadRequest,
			exceptionHTTPTurnBodyTooLarge, oversizeBodyMessage)
		return false
	}
	if field, unknown := unknownJSONField(err); unknown {
		refuse(writer, request, http.StatusBadRequest,
			exceptionHTTPTurnUnknownField, "request body carries an unknown field: "+field)
		return false
	}
	refuse(writer, request, http.StatusBadRequest,
		exceptionHTTPTurnInvalidJSON, "request body must be a JSON object")
	return false
}

// httpPrincipal names the per-user limiter key for an HTTP caller. Callers that
// identify themselves get their own budget; the rest share one.
func httpPrincipal(request *http.Request) string {
	if caller := strings.TrimSpace(request.Header.Get("X-Sirens-Caller")); caller != "" {
		return "http:" + cleanTranscriptText(caller, 64)
	}
	return "http:anonymous"
}

// oversizeBody separates a tripped body cap from a caller whose JSON is wrong.
// Both reach the decoder, and only one is the caller's mistake to fix.
func oversizeBody(err error) bool {
	var tooLarge *http.MaxBytesError
	return errors.As(err, &tooLarge)
}

// unknownJSONField reads the field name out of the decoder's own error, so the
// name comes from the struct rather than from a list that would drift from it.
func unknownJSONField(err error) (string, bool) {
	const prefix = "json: unknown field "
	message := err.Error()
	if !strings.HasPrefix(message, prefix) {
		return "", false
	}
	return strings.Trim(strings.TrimPrefix(message, prefix), `"`), true
}

func (a *Agent) writeHTTPError(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	code exceptionCode,
	message string,
) {
	a.refuseHTTP(writer, request, "http.turn.refused", status, code, message)
}

// writeJobHTTPError is the same record under the jobs event name, so a job
// refusal is not counted as a turn refusal. See sirens-echo#383.
func (a *Agent) writeJobHTTPError(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	code exceptionCode,
	message string,
) {
	a.refuseHTTP(writer, request, "http.job.refused", status, code, message)
}

func (a *Agent) refuseHTTP(
	writer http.ResponseWriter,
	request *http.Request,
	event string,
	status int,
	code exceptionCode,
	message string,
) {
	a.telemetry.MarkSpanError(trace.SpanFromContext(request.Context()), code)
	// The span carried this and no log line did, so an alert reading logs saw
	// nothing for a caller error. See docs/sirens-echo-config.md.
	spec := exceptionFor(code)
	a.telemetry.Error(
		request.Context(),
		event,
		slog.Int("status", status),
		slog.String("error_type", spec.typeName),
		slog.String("stage", spec.stage),
		slog.String("outcome", spec.outcome),
		slog.String("fault", spec.fault),
	)
	http.Error(writer, message, status)
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

// httpTurn also serves the MCP tool, which is the same direct turn reached
// through a different ingress. Only the transport label differs.
type httpTurn struct {
	// trusted records whether the caller authenticated. Only telemetry reads
	// it. See docs/sirens-echo-http.md.
	trusted   bool
	requestID string
	requester string
	transport string
	history   []TranscriptEntry
	current   TranscriptEntry
	reply     string
	reaction  string
}

// RecordReaction keeps the key beside the glyph reply, since this transport
// cannot place a mark.
func (t *httpTurn) RecordReaction(key string) { t.reaction = key }

func (t *httpTurn) RequestID() string { return t.requestID }

func (t *httpTurn) Requester() string { return t.requester }

func (t *httpTurn) Transport() string {
	if t.transport == "" {
		return transportHTTP
	}
	return t.transport
}

func (t *httpTurn) Current() TranscriptEntry { return t.current }

func (t *httpTurn) History(context.Context) ([]TranscriptEntry, error) {
	return append([]TranscriptEntry(nil), t.history...), nil
}

func (t *httpTurn) Reply(_ context.Context, content string) error {
	if t.reply != "" {
		return errors.New("HTTP turn reply already set")
	}
	t.reply = content
	return nil
}

func (a *Agent) resolvePrompt(
	ctx context.Context,
	selected *httpPromptRequest,
) ([]PromptMessage, error) {
	if a.tools == nil {
		return nil, fmt.Errorf("no MCP roster is configured")
	}
	if strings.TrimSpace(selected.Server) == "" || strings.TrimSpace(selected.Name) == "" {
		return nil, PromptRequestError{"prompt requires a server and a name"}
	}
	messages, err := a.tools.Prompt(ctx, selected.Server, selected.Name, selected.Arguments)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, PromptRequestError{fmt.Sprintf(
			"prompt %s/%s returned no readable messages", selected.Server, selected.Name)}
	}
	return messages, nil
}

// assertedHistory copies caller-supplied history and marks its provenance. A
// caller can author an entry as anyone, including this service.
func assertedHistory(supplied []TranscriptEntry) []TranscriptEntry {
	history := make([]TranscriptEntry, 0, len(supplied))
	for _, entry := range supplied {
		entry.Asserted = true
		history = append(history, entry)
	}
	return history
}

// seedFromPrompt folds a prompt into the turn as conversation, which its roles
// already are. With no caller content, its last user message is the request.
func seedFromPrompt(
	history []TranscriptEntry,
	current TranscriptEntry,
	messages []PromptMessage,
) ([]TranscriptEntry, TranscriptEntry) {
	tail := len(messages)
	if strings.TrimSpace(current.Content) == "" &&
		messages[tail-1].Role == "user" {
		current.Content = messages[tail-1].Text
		tail--
	}
	for _, message := range messages[:tail] {
		author := current.Author
		if message.Role == "assistant" {
			author = "assistant"
		}
		history = append(history, TranscriptEntry{Author: author, Content: message.Text})
	}
	return history, current
}
