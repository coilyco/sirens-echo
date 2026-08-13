package community

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
)

// A read-only fetch, bounded by an allowlist the deployment supplies. See
// docs/sirens-echo-fetch.md.

const (
	fetchToolServer = "fetch"
	fetchToolName   = "fetch_url"
)

// FetchProvider exposes an HTTPS GET as a tool. An empty Hosts offers no tool
// at all, rather than a tool that refuses everything.
type FetchProvider struct {
	Hosts  []string
	Client *http.Client
}

// Open needs no per-turn state. The bound is the allowlist, not the requester.
func (p *FetchProvider) Open(context.Context) (ToolSession, error) {
	if p == nil || len(p.Hosts) == 0 {
		return &fetchSession{}, nil
	}
	client := p.Client
	if client == nil {
		client = newFetchClient()
	}
	return &fetchSession{hosts: p.Hosts, client: client}, nil
}

type fetchSession struct {
	hosts  []string
	client *http.Client
}

func (s *fetchSession) Close() error { return nil }

// Grounding is empty: a fetch answers a question rather than volunteering
// reference material the way a server's resources do.
func (s *fetchSession) Grounding() []GroundingDocument { return nil }

func (s *fetchSession) Unavailable() []string { return nil }

func (s *fetchSession) Tools() []ToolDefinition {
	if len(s.hosts) == 0 {
		return nil
	}
	return []ToolDefinition{{
		Name:     fetchToolName,
		Original: fetchToolName,
		Server:   fetchToolServer,
		Description: "Fetch a page over HTTPS and return its text. Only these hosts " +
			"are reachable: " + strings.Join(s.hosts, ", ") + ". Nothing else is.",
		InputSchema: scratchObjectSchema(map[string]any{
			"url": scratchStringProperty("Full https URL to fetch."),
		}, []string{"url"}),
	}}
}

func (s *fetchSession) Call(
	ctx context.Context,
	name string,
	arguments map[string]any,
) (ToolResult, error) {
	if len(s.hosts) == 0 || name != fetchToolName {
		return ToolResult{}, fmt.Errorf("model requested unavailable fetch tool %q", name)
	}
	target, err := s.allowedURL(scratchStringArg(arguments, "url"))
	if err != nil {
		return ToolResult{Text: err.Error(), IsError: true}, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return ToolResult{Text: "that url cannot be requested", IsError: true}, nil
	}
	response, err := s.client.Do(request)
	if err != nil {
		return ToolResult{Text: "that host did not answer", IsError: true}, nil
	}
	defer func() { _ = response.Body.Close() }()
	// One byte past the cap, so a page that fits is distinguishable from one
	// that does not. See docs/sirens-echo-fetch.md.
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(maxFetchBytes)+1))
	if err != nil {
		return ToolResult{Text: "that response could not be read", IsError: true}, nil
	}
	return ToolResult{Text: fetchText(response.StatusCode, body)}, nil
}

// fetchText marks a page that was cut, because a half document the model cannot
// tell from a whole one is answered from with ordinary confidence.
func fetchText(status int, body []byte) string {
	if len(body) <= maxFetchBytes {
		return fmt.Sprintf("%d\n%s", status, string(body))
	}
	// Cutting on a byte offset can split a rune, so the seam is repaired
	// rather than handed to the model broken.
	kept := strings.ToValidUTF8(string(body[:maxFetchBytes]), "")
	return fmt.Sprintf(
		"%d\n%s\n\n[truncated at %d bytes, this page is longer than that]",
		status, kept, maxFetchBytes,
	)
}

// allowedURL refuses anything the allowlist does not name. The host is matched
// exactly, so a lookalike domain cannot pass as a suffix.
func (s *fetchSession) allowedURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("that url cannot be parsed")
	}
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("only https is available, not %q", parsed.Scheme)
	}
	host := strings.ToLower(parsed.Hostname())
	for _, allowed := range s.hosts {
		if hostAllowed(host, allowed) {
			return parsed.String(), nil
		}
	}
	return "", fmt.Errorf("%s is not a host this service may reach", host)
}

// hostAllowed matches an entry against a hostname. A leading *. covers
// subdomains and nothing else. See docs/sirens-echo-fetch.md.
func hostAllowed(host, allowed string) bool {
	// Both sides normalised here, so the predicate is correct on its own terms
	// rather than depending on a caller. See sirens-echo#668.
	host = strings.ToLower(strings.TrimSpace(host))
	allowed = strings.ToLower(strings.TrimSpace(allowed))
	if host == "" || allowed == "" {
		return false
	}
	suffix, wildcard := strings.CutPrefix(allowed, "*.")
	if !wildcard {
		return host == allowed
	}
	// A bare "*." or a pattern with another star is a typo. Refusing beats
	// matching everything, which is what a suffix test would do here.
	if suffix == "" || strings.Contains(suffix, "*") {
		return false
	}
	// The dot is part of the comparison, so notmozilla.com cannot match
	// *.mozilla.com and the apex is a separate entry.
	if !strings.HasSuffix(host, "."+suffix) {
		return false
	}
	// Every label before the suffix has to be real. ".mozilla.com" names no
	// host, and admitting it leaves the allowlist relying on the resolver.
	prefix := host[:len(host)-len(suffix)-1]
	return prefix != "" && !strings.HasPrefix(prefix, ".") &&
		!strings.Contains(prefix, "..")
}

// newFetchClient refuses a private destination at dial time. Checking the
// hostname is not enough. See docs/sirens-echo-fetch.md.
func newFetchClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: fetchTimeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			return refusePrivateAddress(address)
		},
	}
	return &http.Client{
		Timeout:   fetchTimeout,
		Transport: &http.Transport{DialContext: dialer.DialContext},
		CheckRedirect: func(request *http.Request, _ []*http.Request) error {
			// A redirect is a second destination the allowlist never saw.
			return fmt.Errorf("refusing a redirect to %s", request.URL.Hostname())
		},
	}
}

// tailnetRange is carrier-grade NAT, which Tailscale assigns from and which
// IsPrivate does not cover. See docs/sirens-echo-fetch.md.
var tailnetRange = func() *net.IPNet {
	_, network, _ := net.ParseCIDR("100.64.0.0/10")
	return network
}()

// refusePrivateAddress rejects loopback, link-local, and private ranges, which
// is where a cluster keeps everything worth reaching.
func refusePrivateAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("unreadable address %q", address)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("unresolved address %q", host)
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() ||
		tailnetRange.Contains(ip) {
		return fmt.Errorf("refusing an internal address")
	}
	return nil
}

// fetchHosts reads the deployment's allowlist. Empty offers no tool.
func fetchHosts(raw string) []string {
	hosts := make([]string, 0)
	for _, field := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			hosts = append(hosts, trimmed)
		}
	}
	return hosts
}
