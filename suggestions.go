package cmd

import (
	"fmt"
	"strings"
)

func VisibleCommandNames(commands Commands) []string {
	if len(commands) == 0 {
		return nil
	}
	names := make([]string, 0, len(commands))
	for _, command := range commands {
		if command == nil || command.Hidden || strings.TrimSpace(command.Name) == "" {
			continue
		}
		names = append(names, strings.TrimSpace(command.Name))
	}
	return names
}

func SuggestCommands(name string, commands Commands) []string {
	return suggestCommand(strings.TrimSpace(name), commands)
}

func UnknownCommandError(name string, commands Commands) error {
	name = strings.TrimSpace(name)
	suggestions := SuggestCommands(name, commands)
	if len(suggestions) == 0 {
		return fmt.Errorf("unknown command %q", name)
	}
	return fmt.Errorf("unknown command %q. Did you mean %s?", name, strings.Join(suggestions, " or "))
}

func UnknownSubcommandError(command string, target string, subcommands Commands) error {
	command = strings.TrimSpace(command)
	target = strings.TrimSpace(target)
	lines := []string{
		fmt.Sprintf("unknown %s target %q", command, target),
	}

	suggestions := SuggestCommands(target, subcommands)
	if len(suggestions) > 0 {
		lines = append(lines, fmt.Sprintf(`did you mean %q?`, suggestions[0]))
	}

	if available := VisibleCommandNames(subcommands); len(available) > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("available %s targets:", command))
		for _, candidate := range available {
			lines = append(lines, "  "+candidate)
		}
	}

	return fmt.Errorf("%s", strings.Join(lines, "\n"))
}

func UsageError(usageLine string) error {
	return fmt.Errorf("usage: %s", strings.TrimSpace(usageLine))
}
