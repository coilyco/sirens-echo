package community

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// A reply must not carry an identifier this process holds. Input framings are
// unbounded and output values are enumerable. See docs/sirens-echo-identifiers.md.

// snowflakePattern matches a Discord ID. A shorter digit run is an ordinary
// number a reply may legitimately contain, such as a port or a count.
var snowflakePattern = regexp.MustCompile(`^\d{17,20}$`)

// opaqueSecretRunes is the shortest opaque value worth guarding. Anything
// shorter is a word rather than a credential.
const opaqueSecretRunes = 20

// IdentifierGuard holds this process's own identifiers, admitted by shape
// rather than by having appeared in configuration.
type IdentifierGuard struct {
	forbidden []string
	// digits holds the numeric identifiers again, for comparison against a
	// reply stripped to digits. See docs/sirens-echo-identifiers.md.
	digits []string
}

// NewIdentifierGuard derives the set from configuration at boot, so it cannot
// drift from what the pod actually holds. See docs/sirens-echo-identifiers.md.
func NewIdentifierGuard(cfg Config, roster []MCPServerDefinition) *IdentifierGuard {
	guard := &IdentifierGuard{}
	// The principal ID reaches no tool that returns it, so it is forbidden
	// unconditionally rather than only when no tool ran.
	guard.addSnowflake(cfg.Principal.UserID)
	// Channel and guild IDs are deliberately absent. They are configured, not
	// secret, and guarding them made a channel link unsayable. See issue 289.
	for _, server := range roster {
		guard.addEndpoint(server.URL)
		// A declared header carries a credential, so it is guarded the way the
		// Discord token is. addOpaque keeps a short one out.
		for _, value := range server.Headers {
			guard.addOpaque(value)
		}
	}
	guard.addEndpoint(cfg.AgentProxyURL)
	guard.addOpaque(cfg.DiscordToken)
	// Longest first, so a reported match names the most specific value rather
	// than a host that happens to prefix an endpoint.
	sort.Slice(guard.forbidden, func(a, b int) bool {
		return len(guard.forbidden[a]) > len(guard.forbidden[b])
	})
	return guard
}

// The handle is deliberately absent. It is a substring of a host tool output
// legitimately returns, and ValidateIdentityClaim already owns it.

func (g *IdentifierGuard) addSnowflake(value string) {
	value = strings.TrimSpace(value)
	if !snowflakePattern.MatchString(value) {
		return
	}
	g.add(value)
	for _, existing := range g.digits {
		if existing == value {
			return
		}
	}
	g.digits = append(g.digits, value)
}

// minEncodedGuardBytes bounds the base64 reading. A short value's encoding is
// short enough to appear in ordinary text.
const minEncodedGuardBytes = 16

// digitsOnly strips everything that is not a digit, so every separator-based
// spelling of a number collapses into one comparison.
func digitsOnly(text string) string {
	var stripped strings.Builder
	for _, current := range text {
		if current >= '0' && current <= '9' {
			stripped.WriteRune(current)
		}
	}
	return stripped.String()
}

// addEndpoint keeps the host and port together. A bare host is a public name
// and a bare port is an ordinary number, so neither is guarded alone.
func (g *IdentifierGuard) addEndpoint(raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.Port() == "" {
		return
	}
	g.add(parsed.Host)
}

func (g *IdentifierGuard) addOpaque(value string) {
	value = strings.TrimSpace(value)
	if len([]rune(value)) >= opaqueSecretRunes {
		g.add(value)
	}
}

func (g *IdentifierGuard) add(value string) {
	for _, existing := range g.forbidden {
		if existing == value {
			return
		}
	}
	g.forbidden = append(g.forbidden, value)
}

// Guarded reports how many identifiers the set holds, so a deployment can see
// the guard is populated without the values reaching a log.
func (g *IdentifierGuard) Guarded() int {
	if g == nil {
		return 0
	}
	return len(g.forbidden)
}

// Validate rejects a reply carrying one of this process's identifiers. The
// error names the class rather than the value, which must not reach a log.
func (g *IdentifierGuard) Validate(reply string) error {
	if g == nil || len(g.forbidden) == 0 {
		return nil
	}
	for _, value := range g.forbidden {
		if strings.Contains(reply, value) {
			return fmt.Errorf("model reply carried a configured identifier")
		}
	}
	// The invariant is the value, not its spelling. Each reading collapses a
	// different encoding. See docs/sirens-echo-principal-check.md.
	if len(g.digits) > 0 {
		readings := []string{
			digitsOnly(reply),
			digitsOnly(spelledToDigits(reply)),
		}
		for _, value := range g.digits {
			reversed := reverseString(value)
			for _, reading := range readings {
				if strings.Contains(reading, value) ||
					strings.Contains(reading, reversed) {
					return fmt.Errorf("model reply carried a configured identifier")
				}
			}
		}
	}
	// Only long values, because base64 of a short one collides with ordinary
	// text far more readily than it catches an exfiltration.
	for _, value := range g.forbidden {
		if len(value) < minEncodedGuardBytes {
			continue
		}
		for _, encoded := range base64Of(value) {
			if strings.Contains(reply, encoded) {
				return fmt.Errorf("model reply carried a configured identifier")
			}
		}
	}
	return nil
}
