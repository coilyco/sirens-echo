package community

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
)

const genericFailureReply = "I could not produce a grounded answer right now. Please try again shortly."

// Agent owns the Sirens Echo Discord session and its two outbound boundaries.
type Agent struct {
	cfg          Config
	session      *discordgo.Session
	completions  CompletionClient
	issues       IssueTracker
	systemPrompt string
	slots        chan struct{}
	seen         *seenMessages
}

// NewAgent builds the independently deployable Sirens Echo runtime.
func NewAgent(cfg Config) (*Agent, error) {
	skillpack, err := LoadSkillpack(cfg.Definition.SkillRoots)
	if err != nil {
		return nil, err
	}
	session, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return nil, fmt.Errorf("discord session: %w", err)
	}
	session.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsMessageContent
	httpClient := &http.Client{Timeout: cfg.RequestTimeout}
	agent := &Agent{
		cfg:     cfg,
		session: session,
		completions: ProxyClient{
			BaseURL:    cfg.AgentProxyURL,
			Model:      cfg.Definition.Model,
			HTTPClient: httpClient,
		},
		issues: ForgejoClient{
			BaseURL:    cfg.ForgejoBaseURL,
			Token:      cfg.ForgejoToken,
			Owner:      cfg.TrackerOwner,
			Repo:       cfg.TrackerRepo,
			HTTPClient: httpClient,
		},
		systemPrompt: BuildSystemPrompt(cfg.Definition, skillpack),
		slots:        make(chan struct{}, 1),
		seen:         newSeenMessages(1024),
	}
	session.AddHandler(agent.onReady)
	session.AddHandler(agent.onMessage)
	return agent, nil
}

// Run opens the Gateway session and blocks until shutdown.
func (a *Agent) Run(ctx context.Context) error {
	if err := a.session.Open(); err != nil {
		return fmt.Errorf("Discord open: %w", err)
	}
	defer a.session.Close()
	<-ctx.Done()
	return nil
}

func (a *Agent) onReady(session *discordgo.Session, ready *discordgo.Ready) {
	log.Printf("%s ready as %s in %s", a.cfg.Definition.Identity, ready.User.Username, a.cfg.Definition.Channel)
}

func (a *Agent) onMessage(session *discordgo.Session, event *discordgo.MessageCreate) {
	message := event.Message
	if message == nil ||
		message.GuildID == "" ||
		message.ChannelID != a.cfg.DiscordChannelID ||
		message.Author == nil ||
		message.Author.Bot ||
		session.State.User == nil ||
		message.Author.ID == session.State.User.ID {
		return
	}
	if !a.isSummoned(session, message) || !a.seen.Add(message.ID) {
		return
	}
	go a.handleMessage(session, message)
}

func (a *Agent) isSummoned(session *discordgo.Session, message *discordgo.Message) bool {
	botID := session.State.User.ID
	for _, mention := range message.Mentions {
		if mention.ID == botID {
			return true
		}
	}
	if message.ReferencedMessage != nil && message.ReferencedMessage.Author != nil {
		return message.ReferencedMessage.Author.ID == botID
	}
	if message.MessageReference == nil || message.MessageReference.MessageID == "" {
		return false
	}
	referenced, err := session.ChannelMessage(message.ChannelID, message.MessageReference.MessageID)
	return err == nil && referenced.Author != nil && referenced.Author.ID == botID
}

func (a *Agent) handleMessage(session *discordgo.Session, message *discordgo.Message) {
	a.slots <- struct{}{}
	defer func() { <-a.slots }()

	_ = session.ChannelTyping(message.ChannelID)
	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.RequestTimeout)
	defer cancel()

	history, err := a.recentHistory(session, message)
	if err != nil {
		log.Printf("Sirens Echo history read failed: %v", err)
		a.reply(session, message, genericFailureReply)
		return
	}
	current := TranscriptEntry{
		Author:  displayName(message),
		Content: message.ContentWithMentionsReplaced(),
	}
	userPrompt := BuildUserPrompt(history, current)
	raw, err := a.completions.Complete(ctx, a.systemPrompt, userPrompt, message.ID)
	if err != nil {
		log.Printf("Sirens Echo inference failed: %v", err)
		a.reply(session, message, genericFailureReply)
		return
	}
	decision, err := ParseDecision(raw)
	if err == nil {
		err = ValidateGrounding(decision, a.systemPrompt+"\n"+userPrompt)
	}
	if err != nil {
		log.Printf("Sirens Echo rejected model output: %v", err)
		a.reply(session, message, genericFailureReply)
		return
	}

	reply := decision.Reply
	if decision.Issue != nil {
		issueURL, issueErr := a.issues.EnsureIssue(ctx, *decision.Issue)
		if issueErr != nil {
			log.Printf("Sirens Echo issue capture failed: %v", issueErr)
		} else {
			reply = appendTrackingLink(reply, issueURL)
		}
	}
	a.reply(session, message, reply)
}

func (a *Agent) recentHistory(session *discordgo.Session, current *discordgo.Message) ([]TranscriptEntry, error) {
	messages, err := session.ChannelMessages(
		current.ChannelID,
		a.cfg.Definition.MaxContextMessages,
		current.ID,
		"",
		"",
	)
	if err != nil {
		return nil, err
	}
	history := make([]TranscriptEntry, 0, len(messages))
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.Author == nil {
			continue
		}
		history = append(history, TranscriptEntry{
			Author:  displayName(message),
			Content: message.ContentWithMentionsReplaced(),
		})
	}
	return history, nil
}

func (a *Agent) reply(session *discordgo.Session, message *discordgo.Message, content string) {
	_, err := session.ChannelMessageSendComplex(message.ChannelID, &discordgo.MessageSend{
		Content:   truncateRunes(content, 1990),
		Reference: message.SoftReference(),
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse:       []discordgo.AllowedMentionType{},
			RepliedUser: false,
		},
	})
	if err != nil {
		log.Printf("Sirens Echo reply failed: %v", err)
	}
}

func displayName(message *discordgo.Message) string {
	if message.Member != nil && strings.TrimSpace(message.Member.Nick) != "" {
		return message.Member.Nick
	}
	if message.Author != nil {
		if strings.TrimSpace(message.Author.GlobalName) != "" {
			return message.Author.GlobalName
		}
		return message.Author.Username
	}
	return "member"
}

func appendTrackingLink(reply, issueURL string) string {
	suffix := "\n\nTracked for review: <" + issueURL + ">"
	return truncateRunes(reply, 1990-len([]rune(suffix))) + suffix
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

type seenMessages struct {
	mu       sync.Mutex
	capacity int
	order    []string
	values   map[string]struct{}
}

func newSeenMessages(capacity int) *seenMessages {
	return &seenMessages{
		capacity: capacity,
		order:    make([]string, 0, capacity),
		values:   make(map[string]struct{}, capacity),
	}
}

func (s *seenMessages) Add(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.values[id]; exists {
		return false
	}
	s.values[id] = struct{}{}
	s.order = append(s.order, id)
	if len(s.order) > s.capacity {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.values, oldest)
	}
	return true
}
