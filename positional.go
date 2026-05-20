package cmd

import (
	"fmt"
	"io"
	"strings"
)

type PositionalArg struct {
	Name       string
	Usage      string
	Required   bool
	Variadic   bool
	Enum       []string
	Example    string
	Completion CompletionFunc
	// Extensions carries custom metadata for integrations and tooling.
	// Slice and map values are cloned when the library copies metadata, but
	// opaque pointer or custom object payloads are shared by reference. Callers
	// that need full isolation should store immutable values or clone payloads
	// themselves before attaching them here.
	Extensions map[string]any
}

func (c *Command) positionalArgForIndex(index int) *PositionalArg {
	if len(c.Positionals) == 0 || index < 0 {
		return nil
	}
	for i, positional := range c.Positionals {
		if positional.Variadic {
			if i <= index {
				return &c.Positionals[i]
			}
			return nil
		}
		if i == index {
			return &c.Positionals[i]
		}
	}
	return nil
}

func (c *Command) validatePositionals(args []string) error {
	if len(c.Positionals) == 0 {
		return nil
	}

	if !c.hasVariadicPositional() && len(args) > len(c.Positionals) {
		return fmt.Errorf("too many arguments: got %d, want at most %d", len(args), len(c.Positionals))
	}

	for i, positional := range c.Positionals {
		if positional.Required && i >= len(args) {
			return fmt.Errorf("missing required argument: %s", positional.Name)
		}
	}

	for index, value := range args {
		positional := c.positionalArgForIndex(index)
		if positional == nil {
			continue
		}
		if len(positional.Enum) == 0 {
			continue
		}
		if !containsString(positional.Enum, value) {
			return fmt.Errorf("invalid value %q for argument %s", value, positional.Name)
		}
	}

	return nil
}

func (c *Command) hasVariadicPositional() bool {
	for _, positional := range c.Positionals {
		if positional.Variadic {
			return true
		}
	}
	return false
}

func printPositionals(out io.Writer, positionals []PositionalArg) {
	if len(positionals) == 0 {
		return
	}

	maxLen := 0
	for _, positional := range positionals {
		if l := len(positionalSynopsis(positional)); l > maxLen {
			maxLen = l
		}
	}

	fmt.Fprintf(out, "Arguments:\n")
	for _, positional := range positionals {
		fmt.Fprintf(out, "  %-*s %s\n", maxLen+2, positionalSynopsis(positional), positionalDescription(positional))
	}
	fmt.Fprintf(out, "\n")
}

func positionalSynopsis(positional PositionalArg) string {
	name := positional.Name
	if name == "" {
		name = "arg"
	}
	if positional.Variadic {
		name += "..."
	}
	return "<" + name + ">"
}

func positionalDescription(positional PositionalArg) string {
	description := positional.Usage
	annotations := make([]string, 0, 4)
	if positional.Required {
		annotations = append(annotations, "required")
	}
	if positional.Variadic {
		annotations = append(annotations, "variadic")
	}
	if len(positional.Enum) > 0 {
		annotations = append(annotations, "choices: "+strings.Join(positional.Enum, ", "))
	}
	if positional.Example != "" {
		annotations = append(annotations, "example: "+positional.Example)
	}
	if len(annotations) == 0 {
		return description
	}
	if description == "" {
		return "[" + strings.Join(annotations, "] [") + "]"
	}
	return description + " [" + strings.Join(annotations, "] [") + "]"
}

func positionalValueCompletions(app *App, command *Command, positional *PositionalArg, args []string, current string) []string {
	if positional == nil {
		return nil
	}

	values := make([]string, 0)
	if len(positional.Enum) > 0 {
		values = append(values, positional.Enum...)
	}
	if positional.Completion != nil {
		values = append(values, positional.Completion(CompletionContext{
			App:        app,
			Command:    command,
			Positional: positional,
			Args:       append([]string(nil), args...),
			Current:    current,
		})...)
	}
	return uniqueSortedPrefixStrings(values, current)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
