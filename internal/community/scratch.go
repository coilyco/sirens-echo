package community

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// One requester's text filesystem, living for one rollout. Deliberately not the
// job Workspace in workspace.go. See docs/sirens-echo-scratchpad.md.

const (
	// scratchPermissions keep a partition readable only by this process, and
	// deny the execute bit that text-only is enforced by rather than described.
	scratchPermissions     = 0o700
	scratchFilePermissions = 0o600
)

// requesterKey carries the turn's requester to a tool layer that is
// process-wide. See docs/sirens-echo-scratchpad.md.
type requesterKey struct{}

// WithRequester marks a context as belonging to one requesting principal.
func WithRequester(ctx context.Context, requesterID string) context.Context {
	trimmed := strings.TrimSpace(requesterID)
	if trimmed == "" {
		return ctx
	}
	return context.WithValue(ctx, requesterKey{}, trimmed)
}

// RequesterFrom reports the requesting principal a context carries.
func RequesterFrom(ctx context.Context) string {
	value, _ := ctx.Value(requesterKey{}).(string)
	return value
}

// ErrNoScratchRequester refuses a scratch turn carrying no principal. Absence
// is denial, matching the admission gate and the grant table.
var ErrNoScratchRequester = errors.New(
	"scratchpad refused: the turn carries no requesting principal to partition by",
)

// ErrNoScratchSession refuses a turn carrying no workspace. Absence is denial
// rather than one shared bucket every conversation writes into.
var ErrNoScratchSession = errors.New(
	"scratchpad refused: the turn carries no session to partition by",
)

// ScratchProvider exposes a per-requester text filesystem as tools. An empty
// Root offers no tools at all, rather than tools that fail.
type ScratchProvider struct {
	Root string
}

// Open partitions the scratchpad for this turn's requester, supplying the
// attribution executionguard.go refuses execution for lacking.
func (p *ScratchProvider) Open(ctx context.Context) (ToolSession, error) {
	if p == nil || strings.TrimSpace(p.Root) == "" {
		return &scratchSession{}, nil
	}
	requester := RequesterFrom(ctx)
	if requester == "" {
		return nil, ErrNoScratchRequester
	}
	session := SessionFrom(ctx)
	if !session.Valid() {
		return nil, ErrNoScratchSession
	}
	// Session over requester, so a thread is shared and the per-requester
	// quota stays measurable. See docs/sirens-echo-scratchpad.md.
	sessionRoot := filepath.Join(p.Root, session.Directory())
	partition := filepath.Join(sessionRoot, scratchPartitionName(requester))
	if err := os.MkdirAll(partition, scratchPermissions); err != nil {
		return nil, fmt.Errorf("prepare scratchpad partition: %w", err)
	}
	return &scratchSession{
		root:        partition,
		sessionRoot: sessionRoot,
		scratchRoot: p.Root,
		requester:   scratchPartitionName(requester),
		enabled:     true,
	}, nil
}

// scratchPartitionName derives a flat directory name from the requester, by
// hashing rather than stripping. See docs/sirens-echo-scratchpad.md.
func scratchPartitionName(requesterID string) string {
	trimmed := strings.TrimSpace(requesterID)
	if trimmed == "" {
		return "unattributed"
	}
	// A caller-asserted identifier reaches this, so two requesters differing
	// only in punctuation must not land in one partition.
	sum := sha256.Sum256([]byte(trimmed))
	return hex.EncodeToString(sum[:16])
}

type scratchSession struct {
	// root is where this requester writes. sessionRoot is what everyone in the
	// session reads, so a thread's members see one workspace.
	root        string
	sessionRoot string
	// scratchRoot is every session, walked to hold the per-requester quota
	// across the sessions one member takes part in.
	scratchRoot string
	requester   string
	enabled     bool
}

func (s *scratchSession) Grounding() []GroundingDocument { return nil }

func (s *scratchSession) Guidance() []ServerGuidance { return nil }

func (s *scratchSession) Unavailable() []string { return nil }

func (s *scratchSession) Close() error { return nil }

// Tools offers nothing when the capability is off, so no schema reaches the
// prompt on a deployment that mounts no scratchpad.
func (s *scratchSession) Tools() []ToolDefinition {
	if !s.enabled {
		return nil
	}
	return []ToolDefinition{
		{
			Name:     "scratch_list",
			Original: "scratch_list",
			Server:   "scratchpad",
			Description: "List files in your scratchpad. The scratchpad is private to " +
				"you and is erased when the service restarts.",
			InputSchema: scratchObjectSchema(map[string]any{
				"path": scratchStringProperty(
					"Directory to list, relative to the scratchpad root. Defaults to the root.",
				),
			}, nil),
		},
		{
			Name:     "scratch_read",
			Original: "scratch_read",
			Server:   "scratchpad",
			Description: "Read a UTF-8 text file from your scratchpad. A long file comes " +
				"back in one bounded piece that says where it stopped, and offset " +
				"continues from there.",
			InputSchema: scratchObjectSchema(map[string]any{
				"path": scratchStringProperty("File to read, relative to the scratchpad root."),
				"offset": scratchIntegerProperty(
					"Byte to start at. Use the offset the previous read reported.",
				),
			}, []string{"path"}),
		},
		{
			Name:     "scratch_write",
			Original: "scratch_write",
			Server:   "scratchpad",
			Description: "Write a UTF-8 text file to your scratchpad, creating or " +
				"replacing it. Text only.",
			InputSchema: scratchObjectSchema(map[string]any{
				"path":    scratchStringProperty("File to write, relative to the scratchpad root."),
				"content": scratchStringProperty("UTF-8 text to store."),
			}, []string{"path", "content"}),
		},
		{
			Name:        "scratch_search",
			Original:    "scratch_search",
			Server:      "scratchpad",
			Description: "Find lines matching a substring across your scratchpad.",
			InputSchema: scratchObjectSchema(map[string]any{
				"query": scratchStringProperty("Substring to look for. Matching is case-insensitive."),
				"path": scratchStringProperty(
					"Directory to search, relative to the scratchpad root. Defaults to the root.",
				),
			}, []string{"query"}),
		},
	}
}

func scratchObjectSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func scratchStringProperty(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func scratchIntegerProperty(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

// scratchIntArg tolerates the number a JSON decoder produces and the string a
// model sometimes sends instead. A value it cannot read is zero, the default.
func scratchIntArg(arguments map[string]any, key string) int {
	switch value := arguments[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0
		}
		return parsed
	}
	return 0
}

// Call runs one scratchpad verb. A refusal comes back as an error result the
// model can read and correct, not as a Go error, which would fail the turn.
func (s *scratchSession) Call(
	ctx context.Context,
	name string,
	arguments map[string]any,
) (ToolResult, error) {
	if !s.enabled {
		return ToolResult{}, fmt.Errorf("model requested unavailable scratchpad tool %q", name)
	}
	if err := ctx.Err(); err != nil {
		return ToolResult{}, err
	}
	switch name {
	case "scratch_list":
		return s.list(scratchStringArg(arguments, "path"))
	case "scratch_read":
		return s.read(scratchStringArg(arguments, "path"), scratchIntArg(arguments, "offset"))
	case "scratch_write":
		return s.write(scratchStringArg(arguments, "path"), scratchStringArg(arguments, "content"))
	case "scratch_search":
		return s.search(scratchStringArg(arguments, "query"), scratchStringArg(arguments, "path"))
	default:
		return ToolResult{}, fmt.Errorf("model requested unavailable scratchpad tool %q", name)
	}
}

func scratchStringArg(arguments map[string]any, key string) string {
	value, _ := arguments[key].(string)
	return value
}

func scratchRefusal(format string, args ...any) (ToolResult, error) {
	return ToolResult{Text: fmt.Sprintf(format, args...), IsError: true}, nil
}

// reservedScratchPath reports a path landing in the reserved directory. It
// cleans first, so where a path lands decides it rather than how it is spelled.
func reservedScratchPath(relative string) bool {
	cleaned := path.Clean("/" + strings.ReplaceAll(strings.TrimSpace(relative), "\\", "/"))
	trimmed := strings.TrimPrefix(cleaned, "/")
	first, _, _ := strings.Cut(trimmed, "/")
	return strings.EqualFold(first, scratchReservedDir) ||
		strings.EqualFold(first, scratchUploadDir)
}

// resolve confines a path to the requester's own subtree, where a write lands.
// resolveShared is the read counterpart.
func (s *scratchSession) resolve(relative string) (string, error) {
	return s.resolveUnder(s.root, relative)
}

// resolveShared reads your own bare path first, then the rest of the session.
// See docs/sirens-echo-scratchpad.md.
func (s *scratchSession) resolveShared(relative string) (string, error) {
	if strings.TrimSpace(s.sessionRoot) == "" {
		return s.resolve(relative)
	}
	// The root of a shared read is the session, not your corner of it.
	if strings.TrimSpace(relative) == "" {
		return s.sessionRoot, nil
	}
	own, err := s.resolveUnder(s.root, relative)
	if err == nil {
		if _, statErr := os.Stat(own); statErr == nil {
			return own, nil
		}
	}
	shared, sharedErr := s.resolveUnder(s.sessionRoot, relative)
	if sharedErr != nil {
		return "", sharedErr
	}
	if _, statErr := os.Stat(shared); statErr == nil {
		return shared, nil
	}
	// Neither exists, so a miss reads as the caller's own path.
	if err != nil {
		return "", err
	}
	return own, nil
}

// sessionRelative shows your files bare and another member's under its owner.
func (s *scratchSession) sessionRelative(target string) (string, error) {
	base := s.sessionRoot
	if strings.TrimSpace(base) == "" {
		base = s.root
	}
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return "", err
	}
	slashed := filepath.ToSlash(rel)
	if s.requester != "" {
		if trimmed := strings.TrimPrefix(slashed, s.requester+"/"); trimmed != slashed {
			return trimmed, nil
		}
	}
	return slashed, nil
}

func (s *scratchSession) resolveUnder(base, relative string) (string, error) {
	trimmed := strings.TrimSpace(relative)
	if trimmed == "" {
		return base, nil
	}
	if filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "/") {
		return "", errors.New("path must be relative to the scratchpad root")
	}
	if strings.ContainsRune(trimmed, 0) {
		return "", errors.New("path contains a null byte")
	}
	// Refused rather than normalized away, which would write elsewhere while
	// reporting the path asked for. See docs/sirens-echo-scratchpad.md.
	for _, segment := range strings.Split(filepath.ToSlash(trimmed), "/") {
		if segment == ".." {
			return "", errors.New("path must not contain a parent segment")
		}
	}
	cleaned := strings.TrimPrefix(path.Clean("/"+filepath.ToSlash(trimmed)), "/")
	if cleaned == "" || cleaned == "." {
		return base, nil
	}
	if strings.Count(cleaned, "/") >= maxScratchDepth {
		return "", fmt.Errorf("path nests deeper than %d directories", maxScratchDepth)
	}
	candidate := filepath.Join(base, filepath.FromSlash(cleaned))
	return candidate, s.confine(base, candidate)
}

// confine rejects a target escaping base once symlinks are followed,
// evaluating the nearest existing ancestor so a new path is still checked.
func (s *scratchSession) confine(base, candidate string) error {
	rootReal, err := filepath.EvalSymlinks(base)
	if err != nil {
		return fmt.Errorf("resolve scratchpad root: %w", err)
	}
	probe := candidate
	for {
		resolved, resolveErr := filepath.EvalSymlinks(probe)
		if resolveErr == nil {
			if !scratchWithinRoot(rootReal, resolved) {
				return errors.New("path escapes the scratchpad root")
			}
			return nil
		}
		if !errors.Is(resolveErr, fs.ErrNotExist) {
			return fmt.Errorf("resolve scratchpad path: %w", resolveErr)
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return errors.New("path escapes the scratchpad root")
		}
		probe = parent
	}
}

func scratchWithinRoot(root, target string) bool {
	if target == root {
		return true
	}
	return strings.HasPrefix(target, root+string(os.PathSeparator))
}

func (s *scratchSession) list(relative string) (ToolResult, error) {
	target, err := s.resolveShared(relative)
	if err != nil {
		return scratchRefusal("%v", err)
	}
	entries := make([]string, 0, 16)
	walkErr := filepath.WalkDir(target, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if len(entries) >= maxScratchEntries {
			return fs.SkipAll
		}
		if p == target {
			return nil
		}
		rel, relErr := s.sessionRelative(p)
		if relErr != nil {
			return relErr
		}
		if d.IsDir() {
			entries = append(entries, filepath.ToSlash(rel)+"/")
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		entries = append(entries, fmt.Sprintf("%s (%d bytes)", filepath.ToSlash(rel), info.Size()))
		return nil
	})
	if errors.Is(walkErr, fs.ErrNotExist) {
		return scratchRefusal("no such directory: %s", scratchDisplayPath(relative))
	}
	if walkErr != nil {
		return ToolResult{}, fmt.Errorf("list scratchpad: %w", walkErr)
	}
	if len(entries) == 0 {
		return ToolResult{Text: "the scratchpad is empty"}, nil
	}
	sort.Strings(entries)
	return ToolResult{Text: strings.Join(entries, "\n")}, nil
}

func (s *scratchSession) read(relative string, offset int) (ToolResult, error) {
	if strings.TrimSpace(relative) == "" {
		return scratchRefusal("path is required")
	}
	if offset < 0 {
		return scratchRefusal("offset cannot be negative")
	}
	target, err := s.resolveShared(relative)
	if err != nil {
		return scratchRefusal("%v", err)
	}
	info, err := os.Stat(target)
	// Models drop the directory from a spill notice's path. See sirens-echo#8325.
	if errors.Is(err, fs.ErrNotExist) && !strings.ContainsAny(strings.TrimSpace(relative), `/\`) {
		if spilled, spillErr := s.resolveShared(scratchReservedDir + "/" + strings.TrimSpace(relative)); spillErr == nil {
			if spilledInfo, statErr := os.Stat(spilled); statErr == nil {
				target, info, err = spilled, spilledInfo, nil
			}
		}
	}
	if errors.Is(err, fs.ErrNotExist) {
		return scratchRefusal("no such file: %s", scratchDisplayPath(relative))
	}
	if err != nil {
		return ToolResult{}, fmt.Errorf("stat scratchpad file: %w", err)
	}
	if info.IsDir() {
		return scratchRefusal("%s is a directory, use scratch_list", scratchDisplayPath(relative))
	}
	if info.Size() > int64(maxScratchFileBytes) {
		return scratchRefusal(
			"%s is %d bytes, over the %d byte read limit",
			scratchDisplayPath(relative), info.Size(), maxScratchFileBytes,
		)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return ToolResult{}, fmt.Errorf("read scratchpad file: %w", err)
	}
	if !utf8.Valid(data) {
		return scratchRefusal("%s is not UTF-8 text", scratchDisplayPath(relative))
	}
	if offset >= len(data) {
		if offset > 0 {
			return scratchRefusal(
				"%s is %d bytes, so offset %d is past its end",
				scratchDisplayPath(relative), len(data), offset,
			)
		}
		return ToolResult{Text: string(data)}, nil
	}
	return scratchReadPiece(relative, data, offset), nil
}

// scratchReadPiece returns what fits in one tool result and says where it
// stopped. See docs/sirens-echo-scratchpad.md.
func scratchReadPiece(relative string, data []byte, offset int) ToolResult {
	rest := data[offset:]
	if len(rest) <= maxToolResultBytes {
		return ToolResult{Text: string(rest)}
	}
	// Cut on a line where there is one nearby, because a piece ending mid-line
	// reads as a truncated fact rather than as a boundary.
	cut := maxToolResultBytes
	if line := bytes.LastIndexByte(rest[:cut], '\n'); line > cut/2 {
		cut = line + 1
	}
	cut = scratchRuneBoundary(rest, cut)
	next := offset + cut
	return ToolResult{Text: string(rest[:cut]) + fmt.Sprintf(
		"\n[%s: bytes %d to %d of %d, read again with offset %d]",
		scratchDisplayPath(relative), offset, next, len(data), next,
	)}
}

// scratchRuneBoundary walks back off a partial rune, so a piece is always
// valid UTF-8 and the next one starts where this one really ended.
func scratchRuneBoundary(data []byte, cut int) int {
	for cut > 0 && !utf8.Valid(data[:cut]) {
		cut--
	}
	return cut
}

// scratchReservedDir holds what the runtime wrote. The model cannot write here,
// so provenance is a property rather than a convention.
const scratchReservedDir = "tool-output"

// scratchUploadDir holds what a member uploaded. Reserved like tool output so
// the model cannot forge one, and separate so it cannot be mistaken for one.
const scratchUploadDir = "uploads"

// WriteReserved writes on the runtime's behalf, into the directory the model is
// refused. See docs/sirens-echo-scratchpad.md.
func (s *scratchSession) WriteReserved(relative, content string) (ToolResult, error) {
	return s.writeAt(relative, content, true)
}

func (s *scratchSession) write(relative, content string) (ToolResult, error) {
	return s.writeAt(relative, content, false)
}

// writeAt refuses a model-authored write into the reserved directory, so a file
// found there was written by the runtime and not by something imitating it.
func (s *scratchSession) writeAt(relative, content string, runtime bool) (ToolResult, error) {
	// No scratchpad means no write. An empty root confines to the cwd.
	if !s.enabled || strings.TrimSpace(s.root) == "" {
		return scratchRefusal("no scratchpad is mounted")
	}
	if !runtime && reservedScratchPath(relative) {
		return scratchRefusal("%s is reserved for runtime output", scratchReservedDir)
	}
	if strings.TrimSpace(relative) == "" {
		return scratchRefusal("path is required")
	}
	if !utf8.ValidString(content) {
		return scratchRefusal("content must be UTF-8 text")
	}
	if len(content) > maxScratchFileBytes {
		return scratchRefusal(
			"content is %d bytes, over the %d byte file limit",
			len(content), maxScratchFileBytes,
		)
	}
	target, err := s.resolve(relative)
	if err != nil {
		return scratchRefusal("%v", err)
	}
	if target == s.root {
		return scratchRefusal("path is required")
	}
	// The bound the volume was sized against, summed across sessions.
	used, err := s.requesterBytes()
	if err != nil {
		return ToolResult{}, err
	}
	replacing := scratchExistingSize(target)
	if projected := used - replacing + int64(len(content)); projected > int64(maxScratchPartitionBytes) {
		return scratchRefusal(
			"writing %s would use %d bytes, over the %d byte scratchpad limit",
			scratchDisplayPath(relative), projected, maxScratchPartitionBytes,
		)
	}
	// The additional bound. It evicts so an active thread stays usable.
	evicted, err := s.makeSessionRoom(int64(len(content)) - replacing)
	if err != nil {
		return ToolResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), scratchPermissions); err != nil {
		return ToolResult{}, fmt.Errorf("prepare scratchpad directory: %w", err)
	}
	if err := os.WriteFile(target, []byte(content), scratchFilePermissions); err != nil {
		return ToolResult{}, fmt.Errorf("write scratchpad file: %w", err)
	}
	written := fmt.Sprintf("wrote %s (%d bytes)", scratchDisplayPath(relative), len(content))
	// Named rather than silent, so a vanished file is explainable.
	if len(evicted) > 0 {
		written += fmt.Sprintf(
			"\nevicted %d older file(s) to stay inside the %d byte session limit: %s",
			len(evicted), maxSessionBytes, strings.Join(evicted, ", "),
		)
	}
	return ToolResult{Text: written}, nil
}

func (s *scratchSession) search(query, relative string) (ToolResult, error) {
	if strings.TrimSpace(query) == "" {
		return scratchRefusal("query is required")
	}
	target, err := s.resolveShared(relative)
	if err != nil {
		return scratchRefusal("%v", err)
	}
	needle := strings.ToLower(query)
	matches := make([]string, 0, 16)
	// Bytes as well as matches. A count alone let a hundred long lines answer
	// with far more than any consumer holds. See docs/sirens-echo-scratchpad.md.
	spent, truncated := 0, false
	walkErr := filepath.WalkDir(target, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if len(matches) >= maxScratchMatches || spent >= maxToolResultBytes {
			truncated = true
			return fs.SkipAll
		}
		if d.IsDir() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil || info.Size() > int64(maxScratchFileBytes) {
			return nil
		}
		data, readErr := os.ReadFile(p)
		if readErr != nil || !utf8.Valid(data) {
			return nil
		}
		rel, relErr := s.sessionRelative(p)
		if relErr != nil {
			return relErr
		}
		for i, line := range strings.Split(string(data), "\n") {
			if len(matches) >= maxScratchMatches || spent >= maxToolResultBytes {
				truncated = true
				return fs.SkipAll
			}
			if strings.Contains(strings.ToLower(line), needle) {
				match := fmt.Sprintf("%s:%d: %s", filepath.ToSlash(rel), i+1,
					scratchMatchLine(strings.TrimSpace(line)))
				matches = append(matches, match)
				spent += len(match) + 1
			}
		}
		return nil
	})
	if errors.Is(walkErr, fs.ErrNotExist) {
		return scratchRefusal("no such directory: %s", scratchDisplayPath(relative))
	}
	if walkErr != nil {
		return ToolResult{}, fmt.Errorf("search scratchpad: %w", walkErr)
	}
	if len(matches) == 0 {
		return ToolResult{Text: "no matches"}, nil
	}
	text := strings.Join(matches, "\n")
	// Said rather than left to look like the whole answer, which is the reading
	// that sends a model on with a partial picture.
	if truncated {
		text += fmt.Sprintf(
			"\n\n[stopped at %d matches or %d bytes, narrow the query for the rest]",
			maxScratchMatches, maxToolResultBytes,
		)
	}
	return ToolResult{Text: text}, nil
}

// scratchMatchLine bounds one hit. A minified file is one line, and without
// this a single match could be the whole result.
func scratchMatchLine(line string) string {
	if len(line) <= maxScratchMatchRunes {
		return line
	}
	return strings.ToValidUTF8(line[:maxScratchMatchRunes], "") + "..."
}

// requesterBytes totals this requester across sessions, so their ceiling
// survives a workspace shared with other members.
func (s *scratchSession) requesterBytes() (int64, error) {
	if strings.TrimSpace(s.scratchRoot) == "" || s.requester == "" {
		return treeBytes(s.root)
	}
	sessions, err := os.ReadDir(s.scratchRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read scratchpad root: %w", err)
	}
	var total int64
	for _, entry := range sessions {
		if !entry.IsDir() {
			continue
		}
		mine := filepath.Join(s.scratchRoot, entry.Name(), s.requester)
		size, sizeErr := treeBytes(mine)
		if sizeErr != nil {
			return 0, sizeErr
		}
		total += size
	}
	return total, nil
}

// sessionFile is one file in the workspace, with the time eviction orders by.
type sessionFile struct {
	path     string
	relative string
	size     int64
	modified time.Time
}

// sessionFiles lists the whole workspace oldest first, which is the order
// eviction removes in.
func (s *scratchSession) sessionFiles() ([]sessionFile, int64, error) {
	root := s.sessionRoot
	if strings.TrimSpace(root) == "" {
		root = s.root
	}
	files := make([]sessionFile, 0, 16)
	var total int64
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		files = append(files, sessionFile{
			path:     p,
			relative: filepath.ToSlash(rel),
			size:     info.Size(),
			modified: info.ModTime(),
		})
		total += info.Size()
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, fmt.Errorf("measure session: %w", err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modified.Before(files[j].modified) })
	return files, total, nil
}

// makeSessionRoom evicts oldest first until the write fits, inside this
// session only. See docs/sirens-echo-scratchpad.md.
func (s *scratchSession) makeSessionRoom(delta int64) ([]string, error) {
	files, total, err := s.sessionFiles()
	if err != nil {
		return nil, err
	}
	evicted := make([]string, 0)
	for _, file := range files {
		if total+delta <= int64(maxSessionBytes) {
			break
		}
		if removeErr := os.Remove(file.path); removeErr != nil {
			return evicted, fmt.Errorf("evict %s: %w", file.relative, removeErr)
		}
		total -= file.size
		evicted = append(evicted, file.relative)
	}
	return evicted, nil
}

// treeBytes totals one directory, reporting an absent one as empty.
func treeBytes(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("measure scratchpad: %w", err)
	}
	return total, nil
}

func (s *scratchSession) partitionBytes() (int64, error) {
	var total int64
	err := filepath.WalkDir(s.root, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("measure scratchpad: %w", err)
	}
	return total, nil
}

func scratchExistingSize(target string) int64 {
	info, err := os.Stat(target)
	if err != nil || info.IsDir() {
		return 0
	}
	return info.Size()
}

// scratchDisplayPath echoes the requested path without the partition prefix,
// which names the requester.
func scratchDisplayPath(relative string) string {
	trimmed := strings.TrimSpace(relative)
	if trimmed == "" {
		return "."
	}
	return filepath.ToSlash(path.Clean(trimmed))
}
