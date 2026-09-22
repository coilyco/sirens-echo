package community

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// A command's parameter schema is an authority boundary, not input validation.
// See docs/sirens-echo-commands.md.

// CommandParameterType is the closed set of argument shapes.
type CommandParameterType string

const (
	ParameterString  CommandParameterType = "string"
	ParameterInteger CommandParameterType = "integer"
	ParameterBoolean CommandParameterType = "boolean"
)

// CommandParameter declares one argument. Everything a caller may send is
// stated here, and anything not stated is refused.
type CommandParameter struct {
	Name        string
	Description string
	Type        CommandParameterType
	Required    bool
	// MaxLength bounds a string. Zero uses defaultParameterMaxLength.
	MaxLength int
	// Choices closes a string to a fixed set, which is the tightest bound
	// available and the right one whenever the value names a thing.
	Choices []string
	// Pattern bounds a string that cannot be a fixed set.
	Pattern string

	compiled *regexp.Regexp
}

// CommandDefinition is one invocable action. A command submits exactly one job
// kind, so the command surface never outgrows the job surface.
type CommandDefinition struct {
	Name        string
	Description string
	// Kind is the job this command submits. Empty for a command that acts on an
	// existing job, or reports the deployment rather than changing it.
	Kind       string
	Parameters []CommandParameter
	// Ephemeral answers the caller alone, because introspection belongs to
	// whoever asked rather than to the channel.
	Ephemeral bool
}

// JobCommands is the closed command set, one reviewed act to add to. mcpServers
// closes /mcp's argument, and empty drops it. See docs/sirens-echo-commands.md.
func JobCommands(mcpServers []string) []CommandDefinition {
	commands := []CommandDefinition{
		{
			Name:        "echo",
			Description: "Submit an echo job, which proves the lifecycle end to end.",
			Kind:        "echo",
			Parameters: []CommandParameter{{
				Name:        "note",
				Description: "A short note recorded with the job.",
				Type:        ParameterString,
				MaxLength:   120,
			}},
		},
		{
			Name:        "job-status",
			Description: "Report a job's state. Defaults to the job this thread is bound to.",
			Parameters:  []CommandParameter{jobIDParameter()},
		},
		{
			Name:        "job-cancel",
			Description: "Ask a job to stop. Defaults to the job this thread is bound to.",
			Parameters:  []CommandParameter{jobIDParameter()},
		},
		{
			Name:        "mcps",
			Description: "List the MCP servers this deployment reaches, and their tools.",
			Ephemeral:   true,
		},
		{
			Name:        "poll",
			Description: "Post a native Discord poll for a question of your own choosing.",
			Parameters:  pollCommandParameters(),
		},
	}
	if len(mcpServers) == 0 {
		return commands
	}
	return append(commands, CommandDefinition{
		Name:        "mcp",
		Description: "Describe one MCP server's tools, which /mcps has no room to.",
		Ephemeral:   true,
		Parameters: []CommandParameter{{
			Name:        "server",
			Description: "Which server to describe.",
			Type:        ParameterString,
			Required:    true,
			// The roster is fixed at image build, so the set is closed and
			// choices is the tightest bound available.
			Choices: mcpServers,
		}},
	})
}

func jobIDParameter() CommandParameter {
	return CommandParameter{
		Name:        "job",
		Description: "A job id. Omit inside a thread bound to one.",
		Type:        ParameterString,
		MaxLength:   64,
		Pattern:     `^job-[a-z0-9]+$`,
	}
}

// Validate checks the declaration itself, so a malformed command fails at
// startup rather than at a caller's first use.
func (c CommandDefinition) Validate() error {
	if !commandNamePattern.MatchString(c.Name) {
		return fmt.Errorf("command name %q must be lowercase with dashes", c.Name)
	}
	if strings.TrimSpace(c.Description) == "" {
		return fmt.Errorf("command %s has no description", c.Name)
	}
	if c.Kind != "" {
		if _, known := JobKinds[c.Kind]; !known {
			return fmt.Errorf("command %s submits unknown job kind %q", c.Name, c.Kind)
		}
	}
	seen := make(map[string]struct{}, len(c.Parameters))
	for _, parameter := range c.Parameters {
		if !commandNamePattern.MatchString(parameter.Name) {
			return fmt.Errorf("command %s has invalid parameter %q", c.Name, parameter.Name)
		}
		if _, duplicate := seen[parameter.Name]; duplicate {
			return fmt.Errorf("command %s declares %q twice", c.Name, parameter.Name)
		}
		seen[parameter.Name] = struct{}{}
		switch parameter.Type {
		case ParameterString, ParameterInteger, ParameterBoolean:
		default:
			return fmt.Errorf("command %s parameter %s has unknown type %q",
				c.Name, parameter.Name, parameter.Type)
		}
		if parameter.Pattern != "" {
			if _, err := regexp.Compile(parameter.Pattern); err != nil {
				return fmt.Errorf("command %s parameter %s has invalid pattern: %w",
					c.Name, parameter.Name, err)
			}
		}
		// A required argument closed to nothing can never be satisfied, so the
		// command would publish and then refuse every call.
		if parameter.Required && parameter.Type == ParameterString &&
			parameter.Choices != nil && len(parameter.Choices) == 0 {
			return fmt.Errorf("command %s parameter %s is closed to no choices",
				c.Name, parameter.Name)
		}
	}
	return nil
}

var commandNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}[a-z0-9]$`)

// BindArguments is the boundary. It rejects anything the schema does not
// declare and returns only declared, bounded values.
func (c CommandDefinition) BindArguments(supplied map[string]string) (map[string]string, error) {
	declared := make(map[string]CommandParameter, len(c.Parameters))
	for _, parameter := range c.Parameters {
		declared[parameter.Name] = parameter
	}
	// An undeclared argument is refused rather than ignored. Ignoring one lets
	// a caller believe it took effect.
	unknown := make([]string, 0)
	for name := range supplied {
		if _, ok := declared[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("command %s does not declare %s",
			c.Name, strings.Join(unknown, ", "))
	}
	bound := make(map[string]string, len(c.Parameters))
	for _, parameter := range c.Parameters {
		value, present := supplied[parameter.Name]
		value = strings.TrimSpace(value)
		if value == "" {
			if parameter.Required {
				return nil, fmt.Errorf("command %s requires %s", c.Name, parameter.Name)
			}
			continue
		}
		if !present {
			continue
		}
		if err := parameter.check(c.Name, value); err != nil {
			return nil, err
		}
		bound[parameter.Name] = value
	}
	return bound, nil
}

// check applies the declared bound to one value.
func (p CommandParameter) check(command, value string) error {
	limit := p.MaxLength
	if limit <= 0 {
		limit = defaultParameterMaxLength
	}
	if len([]rune(value)) > limit {
		return fmt.Errorf("command %s parameter %s exceeds %d characters", command, p.Name, limit)
	}
	switch p.Type {
	case ParameterInteger:
		if !integerPattern.MatchString(value) {
			return fmt.Errorf("command %s parameter %s must be an integer", command, p.Name)
		}
	case ParameterBoolean:
		if value != "true" && value != "false" {
			return fmt.Errorf("command %s parameter %s must be true or false", command, p.Name)
		}
	case ParameterString:
		if len(p.Choices) > 0 {
			for _, choice := range p.Choices {
				if choice == value {
					return nil
				}
			}
			return fmt.Errorf("command %s parameter %s is not one of its choices", command, p.Name)
		}
		if p.Pattern != "" {
			compiled := p.compiled
			if compiled == nil {
				compiled = regexp.MustCompile(p.Pattern)
			}
			if !compiled.MatchString(value) {
				return fmt.Errorf("command %s parameter %s does not match its pattern", command, p.Name)
			}
		}
	}
	return nil
}

// LookupCommand finds a declared command by name, against the same roster the
// registration used, so a published argument binds the way it was advertised.
func LookupCommand(name string, mcpServers []string) (CommandDefinition, bool) {
	for _, command := range JobCommands(mcpServers) {
		if command.Name == name {
			return command, true
		}
	}
	return CommandDefinition{}, false
}

var integerPattern = regexp.MustCompile(`^-?[0-9]{1,18}$`)
