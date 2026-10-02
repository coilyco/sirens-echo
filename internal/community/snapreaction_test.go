package community

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coilyco/sirens-echo/internal/community/systemone"
)

func TestSnapReactionTakesOnlyAConfidentSocialMark(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		decision RouteDecision
		want     string
	}{
		{"social mark above cutoff", RouteDecision{Ran: true, Shape: "react:wave", ShapeProb: 0.9}, "wave"},
		{"below cutoff", RouteDecision{Ran: true, Shape: "react:wave", ShapeProb: 0.5}, ""},
		{"a fact needs a tool first", RouteDecision{Ran: true, Shape: "react:agree", ShapeProb: 0.99}, ""},
		{"full answer", RouteDecision{Ran: true, Shape: "full", ShapeProb: 0.99}, ""},
		{"a phrase is not a mark", RouteDecision{Ran: true, Shape: "phrase:not-permitted", ShapeProb: 0.99}, ""},
		{"stage never ran", RouteDecision{Shape: "react:wave", ShapeProb: 0.9}, ""},
		{"unknown key", RouteDecision{Ran: true, Shape: "react:shrug", ShapeProb: 0.9}, ""},
	}
	for _, c := range cases {
		key, ok := c.decision.SnapReaction()
		if key != c.want || ok != (c.want != "") {
			t.Errorf("%s: SnapReaction() = %q, %v, want %q", c.name, key, ok, c.want)
		}
	}
	fell := RouteDecision{Ran: true, Shape: "react:wave", ShapeProb: 0.9}
	fell.fellBackTo(RouteFamilyShape, jevFallbackMissingAnswer)
	if _, ok := fell.SnapReaction(); ok {
		t.Error("a fallen-back shape family snapped")
	}
}

func TestEverySnapKeyHasAGlyphAndAMeaning(t *testing.T) {
	t.Parallel()
	for key := range snapReactions {
		if replyReactions[key] == "" || reactionMeanings[key] == "" {
			t.Errorf("snap key %q lacks a glyph or a meaning", key)
		}
	}
	for key := range replyReactions {
		if reactionMeanings[key] == "" {
			t.Errorf("reaction %q has no meaning, so neither the model nor Jev can pick it for one", key)
		}
	}
}

// modelMustNotRun fails the test when the answer path reaches the model.
type modelMustNotRun struct{ t *testing.T }

func (c modelMustNotRun) Complete(context.Context, TurnPrompt, string) (CompletionResult, error) {
	c.t.Error("the model ran on a turn Jev snapped")
	return CompletionResult{Content: "words"}, nil
}

func snappingAgent(t *testing.T, shape string, probability float64) *Agent {
	t.Helper()
	return jevAnsweringAgent(t, map[string]systemone.Answer{
		"shape": {Key: "shape", Option: shape, Probability: probability},
	})
}

// jevAnsweringAgent runs turns against a Jev that answers only the given keys.
func jevAnsweringAgent(t *testing.T, answers map[string]systemone.Answer) *Agent {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req systemone.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}
		resp := systemone.Response{}
		for _, q := range req.Questions {
			if answer, ok := answers[q.Key]; ok {
				resp.Answers = append(resp.Answers, answer)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(server.Close)
	agent := silentTurnAgent(modelMustNotRun{t: t})
	agent.cfg.JevModel = "jev-latest"
	agent.cfg.AgentProxyURL = server.URL
	return agent
}

func TestAJevSnappedTurnMarksTheMessageWithNoModelCall(t *testing.T) {
	agent := snappingAgent(t, "react:heart", 0.95)
	turn := &markableTurn{}

	if err := agent.runTurn(context.Background(), turn, nil); err != nil {
		t.Fatalf("runTurn: %v", err)
	}
	if !marked(turn, replyReactions["heart"]) {
		t.Errorf("marks = %v, want %q", turn.applied, replyReactions["heart"])
	}
	if len(turn.replies) != 0 {
		t.Errorf("replies = %v, want none beside the mark", turn.replies)
	}
}

// The turn tool cannot place a mark, so it returns the glyph and names the key.
func TestAJevSnapOverHTTPReturnsTheGlyphAndTheKey(t *testing.T) {
	agent := snappingAgent(t, "react:wave", 0.95)
	turn := &httpTurn{requestID: "snap", current: TranscriptEntry{Author: "member", Content: "hi"}}

	if err := agent.runTurn(context.Background(), turn, nil); err != nil {
		t.Fatalf("runTurn: %v", err)
	}
	if turn.reply != replyReactions["wave"] || turn.reaction != "wave" {
		t.Errorf("reply, reaction = %q, %q, want %q, wave", turn.reply, turn.reaction, replyReactions["wave"])
	}
}

// A model-invoked mark reports its key the same way, so a caller reads one field.
func TestAModelInvokedMarkOverHTTPNamesTheKey(t *testing.T) {
	t.Parallel()
	agent := silentTurnAgent(answeringClient{reply: "{{react:agree}}"})
	turn := &httpTurn{requestID: "model-mark", current: TranscriptEntry{Author: "member", Content: "is the server up?"}}

	if err := agent.runTurn(context.Background(), turn, nil); err != nil {
		t.Fatalf("runTurn: %v", err)
	}
	if turn.reaction != "agree" || !strings.Contains(turn.reply, replyReactions["agree"]) {
		t.Errorf("reply, reaction = %q, %q, want the agree glyph and key", turn.reply, turn.reaction)
	}
}

func TestARequestedMarkWinsAndMayBeAFact(t *testing.T) {
	t.Parallel()
	social := RouteDecision{Ran: true, Shape: "react:wave", ShapeProb: 0.9}
	cases := []struct {
		name     string
		decision RouteDecision
		want     string
	}{
		{"requested thumbs up", RouteDecision{Ran: true, Requested: "agree", RequestedProb: 0.9, Shape: "full", ShapeProb: 0.9}, "agree"},
		{"request beats shape", RouteDecision{Ran: true, Requested: "heart", RequestedProb: 0.8, Shape: "react:wave", ShapeProb: 0.9}, "heart"},
		{"weak request defers to shape", RouteDecision{Ran: true, Requested: "agree", RequestedProb: 0.4, Shape: "react:wave", ShapeProb: 0.9}, "wave"},
		{"no request", social, "wave"},
		{"unknown requested key", RouteDecision{Ran: true, Requested: "shrug", RequestedProb: 0.99}, ""},
	}
	for _, c := range cases {
		if key, _ := c.decision.SnapReaction(); key != c.want {
			t.Errorf("%s: SnapReaction() = %q, want %q", c.name, key, c.want)
		}
	}
	fell := RouteDecision{Ran: true, Requested: "agree", RequestedProb: 0.99}
	fell.fellBackTo(RouteFamilyRequest, jevFallbackMissingAnswer)
	if key, ok := fell.SnapReaction(); ok {
		t.Errorf("a fallen-back request snapped %q", key)
	}
}

func TestTheRequestQuestionOffersEveryKeyAndNone(t *testing.T) {
	t.Parallel()
	agent := testJevAgent(t)
	questions, _ := agent.buildRouteQuestions(TranscriptEntry{Content: "react with a thumbs up?"}, nil, false, false)
	for _, q := range questions {
		if q.Key != "request" {
			continue
		}
		names := map[string]bool{}
		for _, c := range q.Criteria {
			names[c.Name] = true
		}
		if !names["none"] {
			t.Error("request question lacks a none option, so every turn would be forced to a mark")
		}
		for _, key := range reactKeys() {
			if !names["react:"+key] {
				t.Errorf("request question cannot pick react:%s", key)
			}
		}
		return
	}
	t.Fatal("no request question was built")
}

// Kai asked "can react with a 👍🏽 ?" and got ✅, because only the model could
// reach agree. A member naming the mark now gets it. sirens-echo#8161.
func TestAMemberAskingForAThumbsUpGetsOne(t *testing.T) {
	agent := jevAnsweringAgent(t, map[string]systemone.Answer{
		"request": {Key: "request", Option: "react:agree", Probability: 0.93},
		"shape":   {Key: "shape", Option: "react:acknowledge", Probability: 0.8},
	})
	turn := &markableTurn{}

	if err := agent.runTurn(context.Background(), turn, nil); err != nil {
		t.Fatalf("runTurn: %v", err)
	}
	if !marked(turn, replyReactions["agree"]) || marked(turn, replyReactions["acknowledge"]) {
		t.Errorf("marks = %v, want %q and not %q", turn.applied, replyReactions["agree"], replyReactions["acknowledge"])
	}
}
