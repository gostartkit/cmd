package cmd

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

var errCompletionUsage = errors.New("completion shell must be one of: bash, zsh, fish, powershell")

func (a *App) runBuiltinCommand(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}

	commands := a.rootSubCommands()
	switch args[0] {
	case "completion":
		if commands.Search("completion") != nil {
			return false, nil
		}
		return true, a.runCompletion(args[1:])
	case "spec":
		if commands.Search("spec") != nil {
			return false, nil
		}
		return true, a.runSpec(args[1:])
	case "docs":
		if commands.Search("docs") != nil {
			return false, nil
		}
		return true, a.runDocs(args[1:])
	case "__complete":
		if commands.Search("__complete") != nil {
			return false, nil
		}
		return true, a.runComplete(args[1:])
	default:
		return false, nil
	}
}

func (a *App) runCompletion(args []string) error {
	out := a.Out
	if out == nil {
		out = io.Discard
	}
	if len(args) != 1 {
		fmt.Fprintln(out, "Usage: "+a.Name+" completion [bash|zsh|fish|powershell]")
		return errCompletionUsage
	}

	shell := args[0]
	var script string
	switch shell {
	case "bash":
		script = bashCompletionScript(a.Name)
	case "zsh":
		script = zshCompletionScript(a.Name)
	case "fish":
		script = fishCompletionScript(a.Name)
	case "powershell":
		script = powershellCompletionScript(a.Name)
	default:
		fmt.Fprintln(out, "Usage: "+a.Name+" completion [bash|zsh|fish|powershell]")
		return errCompletionUsage
	}

	_, err := fmt.Fprint(out, script)
	return err
}

func (a *App) runComplete(args []string) error {
	out := a.Out
	if out == nil {
		out = io.Discard
	}
	for _, suggestion := range a.complete(args) {
		fmt.Fprintln(out, suggestion)
	}
	return nil
}

func (a *App) complete(args []string) []string {
	root := a.rootCommand()
	current := ""
	completed := args
	if len(args) > 0 {
		current = args[len(args)-1]
		completed = args[:len(args)-1]
	}

	rootFlags := a.newRootFlagSet()
	currentFlags := rootFlags
	currentCommands := root.SubCommands
	var currentCommand *Command
	var expectingValue *Flag
	afterDoubleDash := false
	positionalArgs := make([]string, 0)

	for _, token := range completed {
		if expectingValue != nil {
			expectingValue = nil
			continue
		}
		if afterDoubleDash {
			positionalArgs = append(positionalArgs, token)
			continue
		}
		if token == "--" {
			afterDoubleDash = true
			continue
		}
		if isFlagToken(token) {
			if flag, consumed, needsValue, _, _ := parseCompletionFlag(currentFlags, token); consumed {
				if needsValue {
					expectingValue = flag
				}
				continue
			}
		}

		cmd := currentCommands.Search(token)
		if cmd == nil {
			positionalArgs = append(positionalArgs, token)
			continue
		}
		currentCommand = cmd
		currentFlags = a.newCommandFlagSet(cmd)
		currentCommands = cmd.SubCommands
	}

	if currentCommand == nil {
		if suggestions, handled := a.completeBuiltin(positionalArgs, current); handled {
			return suggestions
		}
	}

	if expectingValue != nil {
		valueCommand := currentCommand
		if valueCommand == nil {
			valueCommand = root
		}
		return valueCompletions(a, valueCommand, expectingValue, positionalArgs, current, "")
	}

	suggestions := make([]string, 0)
	if current == "" {
		suggestions = append(suggestions, commandCompletions(currentCommands)...)
		if currentCommand == nil {
			suggestions = append(suggestions, a.builtinSpecs()...)
		}
		suggestions = append(suggestions, flagCompletions(currentFlags)...)
		target := currentCommand
		if target == nil && root.Runnable() {
			target = root
		}
		if target != nil {
			suggestions = append(suggestions, positionalValueCompletions(a, target, target.positionalArgForIndex(len(positionalArgs)), positionalArgs, "")...)
		}
		return uniqueSortedStrings(suggestions)
	}

	if strings.HasPrefix(current, "-") {
		if flag, consumed, needsValue, attachedValue, hasAttachedValue := parseCompletionFlag(currentFlags, current); consumed && (needsValue || hasAttachedValue) {
			prefix := ""
			if hasAttachedValue {
				prefix = current[:len(current)-len(attachedValue)]
			}
			valueCommand := currentCommand
			if valueCommand == nil {
				valueCommand = root
			}
			return valueCompletions(a, valueCommand, flag, positionalArgs, attachedValue, prefix)
		}
		return filterPrefix(flagCompletions(currentFlags), current)
	}

	commandSuggestions := filterPrefix(commandCompletions(currentCommands), current)
	if currentCommand == nil {
		commandSuggestions = append(commandSuggestions, filterPrefix(a.builtinSpecs(), current)...)
		commandSuggestions = uniqueSortedStrings(commandSuggestions)
	}
	if len(commandSuggestions) > 0 {
		return commandSuggestions
	}

	target := currentCommand
	if target == nil && root.Runnable() {
		target = root
	}
	if target == nil {
		return nil
	}
	return positionalValueCompletions(a, target, target.positionalArgForIndex(len(positionalArgs)), positionalArgs, current)
}

func (a *App) completeBuiltin(args []string, current string) ([]string, bool) {
	if len(args) == 0 {
		return nil, false
	}

	switch args[0] {
	case "completion":
		if len(args) == 1 {
			return filterPrefix([]string{"bash", "zsh", "fish", "powershell"}, current), true
		}
		return nil, true
	case "spec":
		if len(args) == 1 {
			return filterPrefix([]string{"json"}, current), true
		}
		return nil, true
	case "docs":
		if len(args) == 1 {
			return filterPrefix([]string{"markdown", "man"}, current), true
		}
		return nil, true
	default:
		return nil, false
	}
}

func (a *App) newRootFlagSet() *FlagSet {
	root := a.rootCommand()
	if a.SetFlags == nil && !a.configEnabled() && (root == nil || root.SetFlags == nil) {
		return nil
	}
	flagSet := NewFlagSet(a.Name, ContinueOnError)
	flagSet.SetOutput(io.Discard)
	a.configureFlagSet(flagSet, root, root)
	return flagSet
}

func (a *App) newCommandFlagSet(cmd *Command) *FlagSet {
	root := a.rootCommand()
	flagSet := NewFlagSet(cmd.Name, ContinueOnError)
	flagSet.SetOutput(io.Discard)
	a.configureFlagSet(flagSet, root, cmd)
	return flagSet
}

func isFlagToken(token string) bool {
	return len(token) > 1 && token[0] == '-'
}

func parseCompletionFlag(flagSet *FlagSet, token string) (*Flag, bool, bool, string, bool) {
	if flagSet == nil {
		return nil, false, false, "", false
	}
	if token == "--" {
		return nil, true, false, "", false
	}

	if strings.HasPrefix(token, "--") {
		name := strings.TrimPrefix(token, "--")
		value := ""
		hasValue := false
		if idx := strings.IndexByte(name, '='); idx >= 0 {
			value = name[idx+1:]
			name = name[:idx]
			hasValue = true
		}
		flag, ok := flagSet.Lookup(name)
		if !ok {
			return nil, false, false, "", false
		}
		if hasValue {
			return flag, true, false, value, true
		}
		if boolFlag, ok := flag.Value.(boolFlag); ok && boolFlag.IsBoolFlag() {
			return flag, true, false, "", false
		}
		return flag, true, true, "", false
	}

	name := strings.TrimPrefix(token, "-")
	value := ""
	hasValue := false
	if idx := strings.IndexByte(name, '='); idx >= 0 {
		value = name[idx+1:]
		name = name[:idx]
		hasValue = true
	}
	flag, ok := flagSet.LookupShort(name)
	if !ok {
		return nil, false, false, "", false
	}
	if hasValue {
		return flag, true, false, value, true
	}
	if boolFlag, ok := flag.Value.(boolFlag); ok && boolFlag.IsBoolFlag() {
		return flag, true, false, "", false
	}
	return flag, true, true, "", false
}

func commandCompletions(commands Commands) []string {
	suggestions := make([]string, 0)
	for _, command := range commands {
		if command.Hidden {
			continue
		}
		suggestions = append(suggestions, command.Name)
		for _, alias := range command.Aliases {
			suggestions = append(suggestions, alias)
		}
	}
	return uniqueSortedStrings(suggestions)
}

func flagCompletions(flagSet *FlagSet) []string {
	flags := visibleFlags(flagSet)
	suggestions := make([]string, 0, len(flags)*2)
	for _, flag := range flags {
		if flag.Shorthand != "" {
			suggestions = append(suggestions, "-"+flag.Shorthand)
		}
		suggestions = append(suggestions, "--"+flag.Name)
	}
	return uniqueSortedStrings(suggestions)
}

func filterPrefix(values []string, prefix string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			filtered = append(filtered, value)
		}
	}
	return uniqueSortedStrings(filtered)
}

func valueCompletions(app *App, command *Command, flag *Flag, args []string, current string, prefix string) []string {
	if flag == nil {
		return nil
	}

	values := make([]string, 0)
	if len(flag.Enum) > 0 {
		values = append(values, flag.Enum...)
	}
	if flag.Completion != nil {
		values = append(values, flag.Completion(CompletionContext{
			App:     app,
			Command: command,
			Flag:    flag,
			Args:    append([]string(nil), args...),
			Current: current,
		})...)
	}

	values = filterPrefix(uniqueSortedStrings(values), current)
	if prefix == "" {
		return values
	}

	prefixed := make([]string, 0, len(values))
	for _, value := range values {
		prefixed = append(prefixed, prefix+value)
	}
	return prefixed
}

func uniqueSortedStrings(values []string) []string {
	unique := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	slices.Sort(unique)
	return unique
}

func shellFuncName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func bashCompletionScript(name string) string {
	funcName := "_" + shellFuncName(name) + "_completion"
	return fmt.Sprintf(`%s() {
    local cur
    cur="${COMP_WORDS[COMP_CWORD]}"
    local args=()
    local i
    for ((i=1; i<COMP_CWORD; i++)); do
        args+=("${COMP_WORDS[i]}")
    done
    local suggestions
    suggestions=$(%q __complete "${args[@]}" "$cur")
    COMPREPLY=($(compgen -W "$suggestions" -- "$cur"))
}

complete -F %s %s
`, funcName, name, funcName, name)
}

func zshCompletionScript(name string) string {
	funcName := "_" + shellFuncName(name) + "_completion"
	return fmt.Sprintf(`#compdef %s

%s() {
    local -a args
    args=("${words[@]:2:$((CURRENT-2))}")
    local -a suggestions
    suggestions=("${(@f)$(%q __complete "${args[@]}" "$PREFIX")}")
    compadd -- "${suggestions[@]}"
}

compdef %s %s
`, name, funcName, name, funcName, name)
}

func fishCompletionScript(name string) string {
	return fmt.Sprintf("complete -c %s -f -a '(%s __complete (commandline -opc)[2..-1] (commandline -ct))'\n", name, name)
}

func powershellCompletionScript(name string) string {
	return fmt.Sprintf(`Register-ArgumentCompleter -Native -CommandName %q -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    $tokens = @()
    foreach ($element in $commandAst.CommandElements | Select-Object -Skip 1) {
        if ($element.Extent.EndOffset -lt $cursorPosition) {
            $tokens += $element.Extent.Text
        }
    }

    %q __complete @tokens $wordToComplete | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
    }
}
`, name, name)
}
