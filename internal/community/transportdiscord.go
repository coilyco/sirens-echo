package community

import (
	"context"
	"fmt"
	"time"
)

// discordTransport is the Discord gateway as a Transport. Start is the block
// Run carried inline, and the gates and the turn stay in the Discord files.
type discordTransport struct {
	agent *Agent
}

func (discordTransport) Name() string { return transportDiscord }

func (t discordTransport) Start(ctx context.Context) (func(), error) {
	a := t.agent
	// Before the gateway opens, so no summon reaches a pool not yet draining.
	if a.lane != nil {
		a.lane.start(a.drain.root())
	}
	closeSession := func() {}
	if a.cfg.DiscordGateway {
		if err := a.session.Open(); err != nil {
			return nil, fmt.Errorf("Discord open: %w", err)
		}
		closeSession = func() { _ = a.session.Close() }
	} else if err := a.connectDiscordREST(); err != nil {
		return nil, err
	}
	// A positive signal, so a quiet guild and a stopped gateway differ.
	a.beats = newHeartbeat(time.Now())
	stopWatching := a.watchGateway(ctx)
	if a.events != nil {
		a.startDiscordQueue(ctx)
	}
	return func() {
		stopWatching()
		closeSession()
	}, nil
}
