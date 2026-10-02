package community

import (
	"context"
	"fmt"
	"strings"

	"github.com/slack-go/slack"
)

// slackIdentity is who the bot token belongs to.
type slackIdentity struct {
	UserID string
	TeamID string
}

// slackMessage is the part of a Slack message a turn reads. It is built per
// turn and dropped with it: Slack's API terms bar storing it long-term.
type slackMessage struct {
	TS      string
	User    string
	BotID   string
	SubType string
	Text    string
}

// slackAPI is the half of Slack's Web API the transport calls, so a turn is
// testable without a workspace.
type slackAPI interface {
	AuthTest(ctx context.Context) (slackIdentity, error)
	Post(ctx context.Context, channel, threadTS, text string) (string, error)
	Update(ctx context.Context, channel, ts, text string) error
	Delete(ctx context.Context, channel, ts string) error
	React(ctx context.Context, channel, ts, name string) error
	Unreact(ctx context.Context, channel, ts, name string) error
	Replies(ctx context.Context, channel, threadTS string, limit int) ([]slackMessage, error)
	History(ctx context.Context, channel, before string, limit int) ([]slackMessage, error)
	DisplayName(ctx context.Context, userID string) (string, error)
}

// slackWebAPI is slackAPI over slack-go.
type slackWebAPI struct {
	client *slack.Client
}

func (w slackWebAPI) AuthTest(ctx context.Context) (slackIdentity, error) {
	response, err := w.client.AuthTestContext(ctx)
	if err != nil {
		return slackIdentity{}, err
	}
	return slackIdentity{UserID: response.UserID, TeamID: response.TeamID}, nil
}

// post options keep model text inert: markup parsing off, no link unfurls.
func slackPostOptions(text, threadTS string) []slack.MsgOption {
	options := []slack.MsgOption{
		slack.MsgOptionText(text, false),
		slack.MsgOptionDisableMarkdown(),
		slack.MsgOptionDisableLinkUnfurl(),
		slack.MsgOptionDisableMediaUnfurl(),
	}
	if threadTS != "" {
		options = append(options, slack.MsgOptionTS(threadTS))
	}
	return options
}

func (w slackWebAPI) Post(ctx context.Context, channel, threadTS, text string) (string, error) {
	_, ts, err := w.client.PostMessageContext(ctx, channel, slackPostOptions(text, threadTS)...)
	return ts, err
}

func (w slackWebAPI) Update(ctx context.Context, channel, ts, text string) error {
	_, _, _, err := w.client.UpdateMessageContext(ctx, channel, ts,
		slack.MsgOptionText(text, false), slack.MsgOptionDisableMarkdown())
	return err
}

func (w slackWebAPI) Delete(ctx context.Context, channel, ts string) error {
	_, _, err := w.client.DeleteMessageContext(ctx, channel, ts)
	return err
}

// slackErrorIs matches Slack's error code, which slack-go returns as the text.
func slackErrorIs(err error, codes ...string) bool {
	if err == nil {
		return false
	}
	for _, code := range codes {
		if strings.Contains(err.Error(), code) {
			return true
		}
	}
	return false
}

func (w slackWebAPI) React(ctx context.Context, channel, ts, name string) error {
	err := w.client.AddReactionContext(ctx, name, slack.ItemRef{Channel: channel, Timestamp: ts})
	if slackErrorIs(err, "already_reacted") {
		return nil
	}
	return err
}

func (w slackWebAPI) Unreact(ctx context.Context, channel, ts, name string) error {
	err := w.client.RemoveReactionContext(ctx, name, slack.ItemRef{Channel: channel, Timestamp: ts})
	if slackErrorIs(err, "no_reaction") {
		return nil
	}
	return err
}

func toSlackMessages(messages []slack.Message) []slackMessage {
	out := make([]slackMessage, 0, len(messages))
	for _, message := range messages {
		out = append(out, slackMessage{
			TS:      message.Timestamp,
			User:    message.User,
			BotID:   message.BotID,
			SubType: message.SubType,
			Text:    message.Text,
		})
	}
	return out
}

func (w slackWebAPI) Replies(
	ctx context.Context, channel, threadTS string, limit int,
) ([]slackMessage, error) {
	messages, _, _, err := w.client.GetConversationRepliesContext(ctx,
		&slack.GetConversationRepliesParameters{
			ChannelID: channel, Timestamp: threadTS, Limit: limit,
		})
	if err != nil {
		return nil, err
	}
	return toSlackMessages(messages), nil
}

func (w slackWebAPI) History(
	ctx context.Context, channel, before string, limit int,
) ([]slackMessage, error) {
	response, err := w.client.GetConversationHistoryContext(ctx,
		&slack.GetConversationHistoryParameters{
			ChannelID: channel, Latest: before, Limit: limit,
		})
	if err != nil {
		return nil, err
	}
	return toSlackMessages(response.Messages), nil
}

func (w slackWebAPI) DisplayName(ctx context.Context, userID string) (string, error) {
	user, err := w.client.GetUserInfoContext(ctx, userID)
	if err != nil {
		return "", err
	}
	if name := strings.TrimSpace(user.Profile.DisplayName); name != "" {
		return name, nil
	}
	if name := strings.TrimSpace(user.RealName); name != "" {
		return name, nil
	}
	if user.Name != "" {
		return user.Name, nil
	}
	return "", fmt.Errorf("slack user %s has no name", userID)
}
