package community

import (
	"context"
	"fmt"
)

// Transport is one chat platform this agent listens on. The turn path never
// sees one. See docs/sirens-echo-transports.md.
type Transport interface {
	// Name is the label turns report through turnIO.Transport.
	Name() string
	// Start connects the platform. The returned stop runs after turns drain.
	Start(ctx context.Context) (stop func(), err error)
}

// transports lists the chat platforms this deployment enables. HTTP is not
// one, because every deployment serves it.
func (a *Agent) transports() []Transport {
	var enabled []Transport
	if a.session != nil {
		enabled = append(enabled, discordTransport{agent: a})
	}
	return enabled
}

// startTransports starts each in order and returns one stop that undoes them
// in reverse. A failed start stops the ones already running.
func startTransports(ctx context.Context, list []Transport) (func(), error) {
	var stops []func()
	stopAll := func() {
		for i := len(stops) - 1; i >= 0; i-- {
			stops[i]()
		}
	}
	for _, transport := range list {
		stop, err := transport.Start(ctx)
		if err != nil {
			stopAll()
			return nil, fmt.Errorf("%s transport: %w", transport.Name(), err)
		}
		stops = append(stops, stop)
	}
	return stopAll, nil
}

// progressSinkProvider is an optional turn capability: the sink a long turn
// narrates through. A turn without it gets no progress line.
type progressSinkProvider interface {
	ProgressSink() TurnProgressSink
}

// A transport is held to turnIO plus the capabilities it can honour. One it
// lacks is left out, never stubbed. See docs/sirens-echo-transports.md.
var (
	_ turnIO = (*discordMessageTurn)(nil)
	_ turnIO = (*httpTurn)(nil)

	_ reactor              = (*discordMessageTurn)(nil)
	_ unreactor            = (*discordMessageTurn)(nil)
	_ typingNotifier       = (*discordMessageTurn)(nil)
	_ replyBudget          = (*discordMessageTurn)(nil)
	_ overflowCarrier      = (*discordMessageTurn)(nil)
	_ attachmentBearer     = (*discordMessageTurn)(nil)
	_ prefillReporter      = (*discordMessageTurn)(nil)
	_ spanTagger           = (*discordMessageTurn)(nil)
	_ interruptible        = (*discordMessageTurn)(nil)
	_ progressSinkProvider = (*discordMessageTurn)(nil)

	_ Transport = discordTransport{}
)
