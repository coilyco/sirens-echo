package community

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/bwmarrin/discordgo"
)

// A slash command is a summon path, so it passes the same gates a mention
// does. See docs/sirens-echo-commands.md.

// discordCommands renders the declared set for registration. Registration
// itself is a write to Discord and is deployment-gated.
func discordCommands(mcpServers []string) ([]*discordgo.ApplicationCommand, error) {
	declared := JobCommands(mcpServers)
	rendered := make([]*discordgo.ApplicationCommand, 0, len(declared))
	for _, command := range declared {
		if err := command.Validate(); err != nil {
			return nil, err
		}
		options := make([]*discordgo.ApplicationCommandOption, 0, len(command.Parameters))
		for _, parameter := range command.Parameters {
			options = append(options, renderParameter(parameter))
		}
		rendered = append(rendered, &discordgo.ApplicationCommand{
			Name:        command.Name,
			Description: command.Description,
			Options:     options,
		})
	}
	return rendered, nil
}

// renderParameter maps the declaration onto Discord's option shape. The schema
// here is the same one the handler binds against, so the two cannot drift.
func renderParameter(parameter CommandParameter) *discordgo.ApplicationCommandOption {
	option := &discordgo.ApplicationCommandOption{
		Name:        parameter.Name,
		Description: parameter.Description,
		Required:    parameter.Required,
		Type:        discordgo.ApplicationCommandOptionString,
	}
	switch parameter.Type {
	case ParameterInteger:
		option.Type = discordgo.ApplicationCommandOptionInteger
	case ParameterBoolean:
		option.Type = discordgo.ApplicationCommandOptionBoolean
	case ParameterString:
		if parameter.MaxLength > 0 {
			option.MaxLength = parameter.MaxLength
		}
		for _, choice := range parameter.Choices {
			option.Choices = append(option.Choices, &discordgo.ApplicationCommandOptionChoice{
				Name:  choice,
				Value: choice,
			})
		}
	}
	return option
}

// onInteraction handles one application command. It gates before it acts, so a
// command reaches no work the same caller's message could not.
func (a *Agent) onInteraction(
	session *discordgo.Session,
	event *discordgo.InteractionCreate,
) {
	if event.Type != discordgo.InteractionApplicationCommand {
		return
	}
	ctx := context.Background()
	data := event.ApplicationCommandData()
	command, declared := LookupCommand(data.Name, a.mcpServerNames())
	if !declared {
		a.respondToCommand(session, event, commandReply{Notice: harnessNotice("unknown command")}, false)
		return
	}
	user := interactionUser(event)
	if user == nil {
		return
	}
	origin := interactionContext(event)
	// The same allowlist a mention passes. A command routed by Discord's own
	// picker still has to be admitted by this deployment.
	decision := a.access.Evaluate(origin, user.ID, interactionRoles(event), nil)
	if !decision.allowed() {
		a.telemetry.RecordAccess(ctx, string(decision.Reason))
		a.respondToCommand(session, event, commandReply{Notice: harnessNotice("not permitted here")}, command.Ephemeral)
		return
	}
	admission := a.limiter.Admit(admissionRequest{
		UserKey:    user.ID,
		ContextKey: origin.Key(),
		Override:   decision.Guild.Overrides(),
	})
	if admission.Outcome.denied() {
		a.telemetry.RecordAdmission(ctx, string(admission.Outcome), transportDiscord)
		a.respondToCommand(session, event, commandReply{Notice: cooldownNotice(admission.RetryAfter)}, command.Ephemeral)
		return
	}
	defer a.limiter.Release()

	arguments, err := command.BindArguments(interactionArguments(data))
	if err != nil {
		a.telemetry.Info(ctx, "command.arguments.refused",
			slog.String("command", command.Name))
		a.respondToCommand(session, event, commandReply{Notice: harnessNotice("invalid command arguments")}, command.Ephemeral)
		return
	}
	reply := a.runCommand(ctx, commandRequest{
		Command:       command,
		Arguments:     arguments,
		Principal:     user.ID,
		Origin:        origin,
		InteractionID: event.ID,
		ThreadID:      threadOrigin(session, origin),
	})
	a.respondToCommand(session, event, reply, command.Ephemeral)
}

// commandRequest is one gated, bound invocation ready to act on.
type commandRequest struct {
	Command       CommandDefinition
	Arguments     map[string]string
	Principal     string
	Origin        summonContext
	InteractionID string
	// ThreadID is set when the command was run inside a thread, so a job
	// submitted here binds to it. See docs/sirens-echo-commands.md.
	ThreadID string
}

// threadOrigin reports the thread a command was run in, or empty for an
// ordinary channel. A channel is not a referent, so only a thread is returned.
func threadOrigin(session *discordgo.Session, origin summonContext) string {
	if session == nil || origin.ChannelID == "" {
		return ""
	}
	channel := resolveChannel(session, origin.ChannelID)
	if channel == nil || !channel.IsThread() {
		return ""
	}
	return origin.ChannelID
}

// commandReply is what a command answers with. Poll is set only for a
// command whose whole answer is a native poll.
type commandReply struct {
	Notice string
	Poll   *discordgo.Poll
}

// runCommand performs the declared action and returns what to answer with.
// Every path returns a reply, so a command never ends in silence.
func (a *Agent) runCommand(ctx context.Context, request commandRequest) commandReply {
	command := request.Command
	// Answered above the jobs guard: none of these three submits a job, so a
	// deployment running with jobs off still has them.
	switch command.Name {
	case "mcps":
		return commandReply{Notice: a.mcpRoster(ctx)}
	case "mcp":
		return commandReply{Notice: a.mcpServer(ctx, request.Arguments["server"])}
	case "poll":
		return runPollCommand(request.Arguments)
	}
	if a.jobs == nil {
		return commandReply{Notice: harnessNotice("jobs are not enabled")}
	}
	switch command.Name {
	case "job-status", "job-cancel":
		id, err := ResolveJobReference(a.jobs.Store, request.Arguments["job"], request.Origin.ChannelID)
		if err != nil {
			return commandReply{Notice: harnessNotice("no job named and no job bound to this thread")}
		}
		if command.Name == "job-status" {
			job, err := a.jobs.Get(id, request.Principal)
			if err != nil {
				return commandReply{Notice: harnessNotice("job not found")}
			}
			return commandReply{Notice: harnessNotice(fmt.Sprintf("job %s is %s", job.ID, job.State))}
		}
		job, err := a.jobs.Cancel(ctx, id, request.Principal)
		if err != nil {
			return commandReply{Notice: harnessNotice("job not found")}
		}
		return commandReply{Notice: harnessNotice(fmt.Sprintf("job %s is %s", job.ID, job.State))}
	}
	job, err := a.jobs.Submit(ctx, Submission{
		Kind:      command.Kind,
		Principal: request.Principal,
		Origin: JobOrigin{
			Transport: transportDiscord,
			ChannelID: request.Origin.ChannelID,
			// An interaction carries no message, so its own id is the unit of
			// "the same request" that a message id would have been.
			MessageID: request.InteractionID,
		},
	})
	if err != nil {
		// Retrying a refusal cannot succeed, so it does not get the notice that
		// invites one. See sirens-echo#825.
		if IsGrantDenial(err) {
			return commandReply{Notice: harnessNotice("you are not permitted to start this job kind")}
		}
		return commandReply{Notice: harnessNotice("job could not be accepted")}
	}
	a.bindJobThread(ctx, job, request.ThreadID)
	return commandReply{Notice: harnessNotice(fmt.Sprintf("job %s submitted", job.ID))}
}

// bindJobThread records the thread a job was started in, so a follow-up there
// needs no id. Best effort: a job must not fail because its binding did not.
func (a *Agent) bindJobThread(ctx context.Context, job Job, threadID string) {
	if threadID == "" {
		return
	}
	if _, err := BindJobToThread(a.jobs.Store, job.ID, threadID); err != nil {
		// The likeliest cause is a thread already bound to an earlier job,
		// which is the documented singularity rather than a fault.
		a.telemetry.Info(ctx, "job.thread.unbound",
			slog.String("job_id", job.ID), slog.String("error", err.Error()))
		return
	}
	a.telemetry.Info(ctx, "job.thread.bound", slog.String("job_id", job.ID))
}

func (a *Agent) respondToCommand(
	session *discordgo.Session,
	event *discordgo.InteractionCreate,
	reply commandReply,
	ephemeral bool,
) {
	if session == nil {
		return
	}
	err := session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:         truncateRunes(reply.Notice, 1990),
			Poll:            reply.Poll,
			AllowedMentions: &discordgo.MessageAllowedMentions{},
			Flags:           ephemeralFlag(ephemeral),
		},
	})
	if err != nil {
		a.telemetry.RecordFailure(context.Background(), "command_reply")
	}
}

// interactionArguments flattens Discord's options into the declared shape. The
// schema decides what survives, not this function.
func interactionArguments(
	data discordgo.ApplicationCommandInteractionData,
) map[string]string {
	arguments := make(map[string]string, len(data.Options))
	for _, option := range data.Options {
		if option == nil {
			continue
		}
		arguments[option.Name] = fmt.Sprintf("%v", option.Value)
	}
	return arguments
}

// interactionRoles reads the roles Discord already sent, so a role grant costs
// no extra API call here either.
func interactionRoles(event *discordgo.InteractionCreate) []string {
	if event.Member == nil {
		return nil
	}
	return event.Member.Roles
}

func interactionUser(event *discordgo.InteractionCreate) *discordgo.User {
	if event.Member != nil && event.Member.User != nil {
		return event.Member.User
	}
	return event.User
}

func interactionContext(event *discordgo.InteractionCreate) summonContext {
	if event.GuildID == "" {
		return summonContext{Kind: contextKindDM, ChannelID: event.ChannelID}
	}
	return summonContext{
		Kind:      contextKindGuild,
		GuildID:   event.GuildID,
		ChannelID: event.ChannelID,
	}
}
