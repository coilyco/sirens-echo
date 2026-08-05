package community

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	readyzPath              = "/readyz"
	defaultReadinessTimeout = 5 * time.Second
	maxReadinessBody        = 16 << 10
)

var logicalRouteSegmentPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

type readinessOutcome string

const (
	readinessReady           readinessOutcome = "ready"
	readinessNotReady        readinessOutcome = "not_ready"
	readinessTimeout         readinessOutcome = "timeout"
	readinessUnknownRoute    readinessOutcome = "unknown_route"
	readinessInvalidResponse readinessOutcome = "invalid_response"
	readinessDependencyError readinessOutcome = "dependency_error"
)

type agentProxyReadinessResponse struct {
	Status string `json:"status"`
	Route  string `json:"route"`
}

func newReadinessHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: http.DefaultTransport,
	}
}

func agentProxyReadinessEndpoint(baseURL, logicalRoute string) (string, error) {
	namespace, alias, found := strings.Cut(logicalRoute, "/")
	if !found || strings.Contains(alias, "/") ||
		!logicalRouteSegmentPattern.MatchString(namespace) ||
		!logicalRouteSegmentPattern.MatchString(alias) {
		return "", fmt.Errorf("Agent Proxy model must be a namespace/alias route")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("Agent Proxy URL must be an absolute HTTP URL")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + readyzPath + "/" +
		url.PathEscape(namespace) + "/" + url.PathEscape(alias)
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (a *Agent) checkRouteReadiness(ctx context.Context) readinessOutcome {
	if a.readinessClient == nil || a.readinessEndpoint == "" || a.readinessRoute == "" {
		return readinessDependencyError
	}
	timeout := a.readinessTimeout
	if timeout <= 0 {
		timeout = defaultReadinessTimeout
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(
		requestCtx,
		http.MethodGet,
		a.readinessEndpoint,
		nil,
	)
	if err != nil {
		return readinessDependencyError
	}
	response, err := a.readinessClient.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return readinessTimeout
		}
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			return readinessTimeout
		}
		return readinessDependencyError
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxReadinessBody+1))
	if err != nil || len(raw) > maxReadinessBody {
		return readinessInvalidResponse
	}
	var payload agentProxyReadinessResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return readinessInvalidResponse
	}
	switch response.StatusCode {
	case http.StatusOK:
		if payload.Status == "ready" && payload.Route == a.readinessRoute {
			return readinessReady
		}
	case http.StatusServiceUnavailable:
		if payload.Status == "not_ready" && payload.Route == a.readinessRoute {
			return readinessNotReady
		}
	case http.StatusNotFound:
		if payload.Status == "unknown_route" && payload.Route == "" {
			return readinessUnknownRoute
		}
	default:
		return readinessDependencyError
	}
	return readinessInvalidResponse
}
