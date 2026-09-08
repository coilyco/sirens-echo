package community

import (
	"context"
	"strings"
	"testing"
)

// scratchIn opens a scratchpad for one requester inside a named session, so a
// test can put the same member in two places.
func scratchIn(t *testing.T, root, requester string, session SessionID) ToolSession {
	t.Helper()
	provider := &ScratchProvider{Root: root}
	opened, err := provider.Open(WithSession(WithRequester(context.Background(), requester), session))
	if err != nil {
		t.Fatalf("open scratchpad: %v", err)
	}
	t.Cleanup(func() { _ = opened.Close() })
	return opened
}

// The regression check sirens-echo#265 asks for. A workspace is shared within a
// thread, so per-requester isolation does not cover one member in two threads.
func TestScratchDoesNotBleedAcrossSessions(t *testing.T) {
	root := t.TempDir()
	const member = "111"
	first := scratchIn(t, root, member, ThreadSession("thread-alpha"))
	second := scratchIn(t, root, member, ThreadSession("thread-beta"))

	callScratch(t, first, "scratch_write", map[string]any{
		"path": "report.txt", "content": "luma report only",
	})

	list := callScratch(t, second, "scratch_list", nil)
	if strings.Contains(list.Text, "report.txt") {
		t.Errorf("another session listed the first's file: %s", list.Text)
	}
	read := callScratch(t, second, "scratch_read", map[string]any{"path": "report.txt"})
	if !read.IsError {
		t.Errorf("another session read the first's file: %s", read.Text)
	}
	found := callScratch(t, second, "scratch_search", map[string]any{"query": "luma report only"})
	if strings.Contains(found.Text, "report.txt") || strings.Contains(found.Text, "luma report only") {
		t.Errorf("another session searched the first's file: %s", found.Text)
	}
}

// A direct message and a thread are different sessions even for one member, so
// the surface a turn arrives on partitions the workspace too.
func TestScratchDoesNotBleedBetweenDirectAndThread(t *testing.T) {
	root := t.TempDir()
	const member = "111"
	direct := scratchIn(t, root, member, DirectSession("channel-a", member))
	thread := scratchIn(t, root, member, ThreadSession("thread-a"))

	callScratch(t, direct, "scratch_write", map[string]any{
		"path": "dm.txt", "content": "said in a direct message",
	})

	found := callScratch(t, thread, "scratch_search", map[string]any{"query": "said in a direct message"})
	if strings.Contains(found.Text, "dm.txt") || strings.Contains(found.Text, "said in a direct message") {
		t.Errorf("a thread turn reached a direct-message workspace: %s", found.Text)
	}
}

// The sharing that IS intended, beside the isolation, so the boundary reads as
// a decision rather than an accident.
func TestScratchIsSharedWithinOneThread(t *testing.T) {
	root := t.TempDir()
	thread := ThreadSession("thread-shared")
	author := scratchIn(t, root, "111", thread)
	reader := scratchIn(t, root, "222", thread)

	callScratch(t, author, "scratch_write", map[string]any{
		"path": "notes.txt", "content": "shared with the thread",
	})

	found := callScratch(t, reader, "scratch_search", map[string]any{"query": "shared with the thread"})
	if !strings.Contains(found.Text, "shared with the thread") {
		t.Errorf("a thread member could not read the thread's workspace: %s", found.Text)
	}
}
