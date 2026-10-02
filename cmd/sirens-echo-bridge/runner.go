package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/coilyco/sirens-echo/internal/coalesce"
	"github.com/coilyco/sirens-echo/internal/community"
)

// Sink is where a turn's visible output goes: the one-line plan posted before
// the work, and the summary that marks every covered ask done.
type Sink interface {
	Plan(ctx context.Context, batch coalesce.Batch, plan string) error
	Done(ctx context.Context, batch coalesce.Batch, summary string) error
}

// proxyRunner answers one batch in one Agent Proxy completion. Thinking and the
// wider model are route aliases, since Agent Proxy owns inference tuning.
type proxyRunner struct {
	base   community.ProxyClient
	direct string
	pro    string
	system string
	sink   Sink
}

// clientFor picks the route this attempt runs on. The ladder tries thinking off
// before the wider model, since a timeout is more often reasoning than power.
func (r *proxyRunner) clientFor(attempt coalesce.Attempt) community.ProxyClient {
	client := r.base
	switch {
	case attempt.Tier == coalesce.TierPro && r.pro != "":
		client.Model = r.pro
	case !attempt.Thinking && r.direct != "":
		client.Model = r.direct
	}
	return client
}

func (r *proxyRunner) Run(ctx context.Context, batch coalesce.Batch) error {
	if err := r.sink.Plan(ctx, batch, planLine(batch)); err != nil {
		return fmt.Errorf("post plan: %w", err)
	}
	prompt := community.BuildTurnPrompt(r.system, nil, community.TranscriptEntry{
		Author:  batch.Tenant.Author,
		Content: batch.Criteria(),
	})
	client := r.clientFor(batch.Attempt)
	result, err := client.Complete(ctx, prompt, requestIDFor(batch))
	if err != nil {
		return err
	}
	if strings.TrimSpace(result.Content) == "" {
		return fmt.Errorf("model returned no content on attempt %d", batch.Attempt.Number)
	}
	return r.sink.Done(ctx, batch, result.Content)
}

// planLine states what the turn will cover before it starts, so a member whose
// comment was folded into a batch can see that it was.
func planLine(batch coalesce.Batch) string {
	covered := make([]string, 0, batch.Size())
	for _, ask := range batch.Asks() {
		covered = append(covered, fmt.Sprintf("#%d", ask.Seq))
	}
	return fmt.Sprintf("working %d comment(s) in one turn: %s",
		batch.Size(), strings.Join(covered, " "))
}

// requestIDFor names the turn by the batch's first ask, so a trace and a
// comment can be lined up without a second identifier.
func requestIDFor(batch coalesce.Batch) string {
	asks := batch.Asks()
	if len(asks) == 0 {
		return "batch-empty"
	}
	return fmt.Sprintf("batch-%d-%d", asks[0].Seq, batch.Attempt.Number)
}

// stubRunner stands in for the model in the smoke feed, which proves the lane
// drains without spending a completion or needing a proxy at all.
type stubRunner struct {
	takes time.Duration
	sink  Sink
}

func (s *stubRunner) Run(ctx context.Context, batch coalesce.Batch) error {
	if err := s.sink.Plan(ctx, batch, planLine(batch)); err != nil {
		return err
	}
	select {
	case <-time.After(s.takes):
	case <-ctx.Done():
		return ctx.Err()
	}
	return s.sink.Done(ctx, batch, fmt.Sprintf("served %d comment(s)", batch.Size()))
}
