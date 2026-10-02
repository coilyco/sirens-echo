package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"

	"github.com/coilyco/sirens-echo/internal/coalesce"
	"github.com/coilyco/sirens-echo/internal/ingest"
)

// record is one line of the bridge's output stream. It is the changelog in the
// form a pipe can read, and the shape a Discord sink would post from.
type record struct {
	Event   string  `json:"event"`
	Seq     int64   `json:"seq,omitempty"`
	Seqs    []int64 `json:"seqs,omitempty"`
	Surface string  `json:"surface,omitempty"`
	Author  string  `json:"author,omitempty"`
	Locus   string  `json:"locus,omitempty"`
	Text    string  `json:"text,omitempty"`
	Attempt int     `json:"attempt,omitempty"`
	Tier    string  `json:"tier,omitempty"`
}

// stream is both the acknowledging surface and the turn's output sink, so the
// per-comment ack and the per-batch summary land in one ordered record.
type stream struct {
	mu  sync.Mutex
	out *bufio.Writer
}

func newStream(w io.Writer) *stream {
	return &stream{out: bufio.NewWriter(w)}
}

func (s *stream) write(entry record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := json.NewEncoder(s.out).Encode(entry); err != nil {
		return err
	}
	return s.out.Flush()
}

// Queued is the individually visible acknowledgment. One per comment, applied
// at ingress, which is the demo rule coalescing is allowed to exist under.
func (s *stream) Queued(_ context.Context, ask ingest.Ask) error {
	return s.write(record{
		Event:   "queued",
		Seq:     ask.Seq,
		Surface: ask.Tenant.Surface,
		Author:  ask.Tenant.Author,
		Locus:   ask.Locus,
	})
}

// Shed retracts that mark, because the ask it promised an answer for is gone.
func (s *stream) Shed(_ context.Context, ask ingest.Ask) error {
	return s.write(record{Event: "shed", Seq: ask.Seq, Surface: ask.Tenant.Surface})
}

func (s *stream) Plan(_ context.Context, batch coalesce.Batch, plan string) error {
	return s.write(record{
		Event:   "plan",
		Seqs:    seqsOf(batch),
		Surface: batch.Tenant.Surface,
		Author:  batch.Tenant.Author,
		Text:    plan,
		Attempt: batch.Attempt.Number,
		Tier:    string(batch.Attempt.Tier),
	})
}

// Done marks every ask the batch covered, one summary for the batch and the
// full list of comments it answers.
func (s *stream) Done(_ context.Context, batch coalesce.Batch, summary string) error {
	return s.write(record{
		Event:   "done",
		Seqs:    seqsOf(batch),
		Surface: batch.Tenant.Surface,
		Author:  batch.Tenant.Author,
		Text:    summary,
		Attempt: batch.Attempt.Number,
		Tier:    string(batch.Attempt.Tier),
	})
}

// Shelve is the dead letter. The member is told the work is still queued rather
// than told nothing, because silence reads as being ignored.
func (s *stream) Shelve(_ context.Context, batch coalesce.Batch, cause error) {
	entry := record{
		Event:   "queued-retrying",
		Seqs:    seqsOf(batch),
		Surface: batch.Tenant.Surface,
		Author:  batch.Tenant.Author,
		Attempt: batch.Attempt.Number,
	}
	if cause != nil {
		entry.Text = cause.Error()
	}
	_ = s.write(entry)
}

func seqsOf(batch coalesce.Batch) []int64 {
	asks := batch.Asks()
	seqs := make([]int64, 0, len(asks))
	for _, ask := range asks {
		seqs = append(seqs, ask.Seq)
	}
	return seqs
}
