package community

import (
	"fmt"
	"strconv"

	"github.com/bwmarrin/discordgo"
)

// /poll posts a member-invoked, free-form native poll. See
// docs/sirens-echo-commands.md.

// pollMinAnswers and pollMinDuration are Discord's structural floor, not a
// deployment's tuning. See knobhome_test.go's elsewhereByDesign.
const (
	pollMinAnswers  = 2
	pollMinDuration = 1
)

// pollAnswerParameterNames is the declared, closed order, so the schema a
// member sees is stable across releases rather than generated fresh.
var pollAnswerParameterNames = []string{
	"answer-1", "answer-2", "answer-3", "answer-4", "answer-5",
	"answer-6", "answer-7", "answer-8", "answer-9", "answer-10",
}

// pollCommandParameters declares /poll's whole argument schema. check() in
// command.go enforces every bound named here.
func pollCommandParameters() []CommandParameter {
	parameters := []CommandParameter{{
		Name:        "question",
		Description: "The poll question.",
		Type:        ParameterString,
		Required:    true,
		MaxLength:   pollQuestionRunes,
	}}
	for index, name := range pollAnswerParameterNames {
		parameters = append(parameters, CommandParameter{
			Name:        name,
			Description: fmt.Sprintf("Answer %d.", index+1),
			Type:        ParameterString,
			// The first two answers are what makes it a poll; Discord itself
			// refuses fewer than two.
			Required:  index < pollMinAnswers,
			MaxLength: pollAnswerRunes,
		})
	}
	return append(parameters,
		CommandParameter{
			Name: "duration-hours",
			Description: fmt.Sprintf(
				"How long the poll runs, %d to %d hours. Defaults to %d.",
				pollMinDuration, pollMaxDuration, pollDefaultDuration,
			),
			Type: ParameterInteger,
		},
		CommandParameter{
			Name:        "multiselect",
			Description: "Let a voter pick more than one answer. Defaults to false.",
			Type:        ParameterBoolean,
		},
	)
}

// pollFromArguments builds the native poll a bound /poll invocation
// describes, adding the one check the schema has no shape for: an integer's range.
func pollFromArguments(arguments map[string]string) (*discordgo.Poll, error) {
	answers := make([]discordgo.PollAnswer, 0, len(pollAnswerParameterNames))
	for _, name := range pollAnswerParameterNames {
		text, supplied := arguments[name]
		if !supplied {
			continue
		}
		answers = append(answers, discordgo.PollAnswer{
			Media: &discordgo.PollMedia{Text: text},
		})
	}
	if len(answers) < pollMinAnswers {
		return nil, fmt.Errorf("a poll needs at least %d answers", pollMinAnswers)
	}
	duration, err := pollDuration(arguments["duration-hours"])
	if err != nil {
		return nil, err
	}
	return &discordgo.Poll{
		Question:         discordgo.PollMedia{Text: arguments["question"]},
		Answers:          answers,
		AllowMultiselect: arguments["multiselect"] == "true",
		LayoutType:       discordgo.PollLayoutTypeDefault,
		Duration:         duration,
	}, nil
}

// pollDuration resolves the optional argument to Discord's hour count,
// defaulting where the member named none. See sirens-echo#8067.
func pollDuration(raw string) (int, error) {
	if raw == "" {
		return pollDefaultDuration, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("duration-hours must be an integer")
	}
	if parsed < pollMinDuration || parsed > pollMaxDuration {
		return 0, fmt.Errorf("duration-hours must be between %d and %d", pollMinDuration, pollMaxDuration)
	}
	return parsed, nil
}

// runPollCommand is /poll's branch of runCommand. A poll is the reply itself,
// so a valid one carries no notice text beside it.
func runPollCommand(arguments map[string]string) commandReply {
	poll, err := pollFromArguments(arguments)
	if err != nil {
		return commandReply{Notice: harnessNotice(err.Error())}
	}
	return commandReply{Poll: poll}
}
