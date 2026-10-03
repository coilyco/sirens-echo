package community

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	httpMessagePath = "/v1/message"
	// One variable per reachable channel, its value the channel id. See
	// docs/sirens-echo-transports.md.
	messageChannelEnvPrefix = "SIRENS_ECHO_MESSAGE_CHANNEL_"
)

// channelSlugShape is a Discord channel name as a caller writes it. Anything
// else cannot name an environment variable, so it is refused before a lookup.
var channelSlugShape = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)

type httpMessageRequest struct {
	Channel string `json:"channel"`
	Content string `json:"content"`
}

type httpMessageResponse struct {
	MessageID string `json:"message_id"`
}

// messageChannelsFromEnv keys the allowlist by normalized slug. A bad id stops
// the boot, since a typo would otherwise surface only on the first post.
func messageChannelsFromEnv(environ []string) (map[string]string, error) {
	channels := map[string]string{}
	for _, entry := range environ {
		name, value, _ := strings.Cut(entry, "=")
		key, found := strings.CutPrefix(name, messageChannelEnvPrefix)
		value = strings.TrimSpace(value)
		if !found || key == "" || value == "" {
			continue
		}
		if !discordSnowflake.MatchString(value) {
			return nil, fmt.Errorf("%s must be a numeric Discord channel id", name)
		}
		channels[key] = value
	}
	return channels, nil
}

// normalizeChannelSlug returns the allowlist key and the slug to name back,
// the slug only when shaped like a channel, so a refusal echoes no stray bytes.
func normalizeChannelSlug(raw string) (key, slug string, ok bool) {
	slug = strings.TrimPrefix(strings.TrimSpace(raw), "#")
	if !channelSlugShape.MatchString(slug) {
		return "", "", false
	}
	return strings.ToUpper(strings.ReplaceAll(slug, "-", "_")), slug, true
}

// writeMessageHTTPError is the same record under the message event name, so a
// refused post is not counted as a refused turn.
func (a *Agent) writeMessageHTTPError(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	code exceptionCode,
	message string,
) {
	a.refuseHTTP(writer, request, "http.message.refused", status, code, message)
}

// handleHTTPMessage posts the content verbatim, so the allowlist and the mention
// policy are the whole boundary. See docs/sirens-echo-transports.md.
func (a *Agent) handleHTTPMessage(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		a.writeMessageHTTPError(writer, request, http.StatusMethodNotAllowed,
			exceptionHTTPTurnMethodNotAllowed, "method not allowed")
		return
	}
	var payload httpMessageRequest
	if !a.decodeHTTPBody(writer, request, a.writeMessageHTTPError, &payload) {
		return
	}
	if strings.TrimSpace(payload.Channel) == "" {
		a.writeMessageHTTPError(writer, request, http.StatusBadRequest,
			exceptionHTTPTurnContentRequired, "channel is required")
		return
	}
	if strings.TrimSpace(payload.Content) == "" {
		a.writeMessageHTTPError(writer, request, http.StatusBadRequest,
			exceptionHTTPTurnContentRequired, "content is required")
		return
	}
	// Refused, never cut: a pass-through that edits what it was handed is not one.
	if len([]rune(payload.Content)) > discordReplyLimit {
		a.writeMessageHTTPError(writer, request, http.StatusBadRequest,
			exceptionHTTPTurnInputTooLong,
			fmt.Sprintf("content exceeds the %d character limit", discordReplyLimit))
		return
	}
	key, slug, shaped := normalizeChannelSlug(payload.Channel)
	channelID := a.cfg.MessageChannels[key]
	if !shaped || channelID == "" {
		message := "channel is not allowed"
		if shaped {
			message = fmt.Sprintf("channel %q is not allowed", slug)
		}
		a.writeMessageHTTPError(writer, request, http.StatusForbidden,
			exceptionJobNotPermitted, message)
		return
	}
	if a.session == nil {
		a.writeMessageHTTPError(writer, request, http.StatusServiceUnavailable,
			exceptionDiscordReplyFailed, "discord is not enabled")
		return
	}
	sent, err := a.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: payload.Content,
		// Never Parse: a caller here is anonymous, and @everyone is one string.
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse: []discordgo.AllowedMentionType{},
		},
	}, discordgo.WithContext(request.Context()))
	if err != nil {
		a.logMessageSendFailure(request, slug, err)
		a.writeMessageHTTPError(writer, request, http.StatusBadGateway,
			exceptionDiscordReplyFailed, "message could not be sent")
		return
	}
	span := trace.SpanFromContext(request.Context())
	span.SetAttributes(
		attribute.String("messaging.system", "discord"),
		attribute.String("messaging.destination.name", slug),
	)
	response := httpMessageResponse{}
	if sent != nil {
		response.MessageID = sent.ID
		span.SetAttributes(attribute.String("messaging.message.id", sent.ID))
	}
	writeJSON(writer, http.StatusOK, response)
}

// logMessageSendFailure keeps Discord's status and code, which say whether the
// bot lost Send Messages or the channel went away, and drops the body.
func (a *Agent) logMessageSendFailure(request *http.Request, slug string, err error) {
	attributes := []slog.Attr{slog.String("channel", slug)}
	var rest *discordgo.RESTError
	if errors.As(err, &rest) {
		if rest.Response != nil {
			attributes = append(attributes, slog.Int("discord_status", rest.Response.StatusCode))
		}
		if rest.Message != nil {
			attributes = append(attributes, slog.Int("discord_code", rest.Message.Code))
		}
	}
	a.telemetry.Error(request.Context(), "http.message.send_failed", attributes...)
}
