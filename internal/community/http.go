package community

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	httpTurnPath = "/v1/turn"
	healthzPath  = "/healthz"
	maxHTTPBody  = 64 << 10
)

type httpTurnRequest struct {
	RequestID string            `json:"request_id"`
	Author    string            `json:"author"`
	Content   string            `json:"content"`
	History   []TranscriptEntry `json:"history"`
}

type httpTurnResponse struct {
	Reply string `json:"reply,omitempty"`
	Error string `json:"error,omitempty"`
}

// HTTPHandler exposes the same turn path as a summoned Discord message after
// Discord-specific gates pass. It is intended for private or loopback tests.
func (a *Agent) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(healthzPath, a.handleHealthz)
	mux.HandleFunc(httpTurnPath, a.handleHTTPTurn)
	return mux
}

func (a *Agent) handleHealthz(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
}

func (a *Agent) handleHTTPTurn(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxHTTPBody)
	var payload httpTurnRequest
	decoder := json.NewDecoder(request.Body)
	if err := decoder.Decode(&payload); err != nil {
		http.Error(writer, "request body must be a JSON object", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(payload.Content) == "" {
		http.Error(writer, "content is required", http.StatusBadRequest)
		return
	}
	if payload.Author == "" {
		payload.Author = "manual test"
	}
	if len([]rune(payload.Author)) > 256 || len([]rune(payload.Content)) > 16000 {
		http.Error(writer, "author or content is too long", http.StatusBadRequest)
		return
	}
	if len(payload.History) > a.cfg.Definition.MaxContextMessages {
		http.Error(writer, "history exceeds the configured context limit", http.StatusBadRequest)
		return
	}
	if payload.RequestID == "" {
		payload.RequestID = fmt.Sprintf("http-%d", time.Now().UnixNano())
	}

	turn := &httpTurn{
		requestID: payload.RequestID,
		history:   append([]TranscriptEntry(nil), payload.History...),
		current: TranscriptEntry{
			Author:  payload.Author,
			Content: payload.Content,
		},
	}
	ctx, cancel := context.WithTimeout(request.Context(), a.cfg.RequestTimeout)
	defer cancel()
	if err := a.runSerialized(ctx, turn); err != nil {
		writeJSON(writer, http.StatusBadGateway, httpTurnResponse{
			Reply: turn.reply,
			Error: "turn failed",
		})
		return
	}
	writeJSON(writer, http.StatusOK, httpTurnResponse{Reply: turn.reply})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

type httpTurn struct {
	requestID string
	history   []TranscriptEntry
	current   TranscriptEntry
	reply     string
}

func (t *httpTurn) RequestID() string { return t.requestID }

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
