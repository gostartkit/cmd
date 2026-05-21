package cmd

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

var errCompletionUsage = errors.New("completion shell must be one of: bash, zsh, fish, powershell")

const (
	completionKindCommand    = "command"
	completionKindFlag       = "flag"
	completionKindValue      = "value"
	completionKindPositional = "positional"
	completionKindBuiltin    = "builtin"
)

type CompletionResult struct {
	Value       string
	Description string
	Kind        string
}

type DetailedLineCompleter interface {
	CompleteLineDetailed(line string, cursor int) []CompletionResult
}

type completionState struct {
	root          *Command
	rootBuiltins  []string
	current       string
	currentFlags  *FlagSet
	currentCmds   Commands
	currentCmd    *Command
	expectingFlag *Flag
	positional    []string
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
	state := a.resolveCompletionState(args)

	if state.currentCmd == nil {
		if suggestions, handled := a.completeBuiltin(state.positional, state.current); handled {
			return suggestions
		}
	}

	if state.expectingFlag != nil {
		valueCommand := state.currentCmd
		if valueCommand == nil {
			valueCommand = state.root
		}
		return valueCompletions(a, valueCommand, state.expectingFlag, state.positional, state.current, "")
	}

	suggestions := make([]string, 0)
	if state.current == "" {
		if state.currentCmd == nil && state.currentFlags == nil && !state.root.Runnable() {
			return a.rootCompletionValues()
		}
		suggestions = make([]string, 0, estimateStringCompletionCapacity(state.currentCmds, state.currentFlags, state.currentCmd == nil, len(state.rootBuiltins)))
		if state.currentCmd == nil {
			suggestions = append(suggestions, a.rootCommandCompletionValues()...)
		} else {
			suggestions = appendCommandCompletionValues(suggestions, state.currentCmds)
		}
		if state.currentCmd == nil {
			suggestions = append(suggestions, state.rootBuiltins...)
		}
		suggestions = appendFlagCompletionValues(suggestions, state.currentFlags)
		target := state.currentCmd
		if target == nil && state.root.Runnable() {
			target = state.root
		}
		if target != nil {
			suggestions = append(suggestions, positionalValueCompletions(a, target, target.positionalArgForIndex(len(state.positional)), state.positional, "")...)
		}
		return uniqueSortedStrings(suggestions)
	}

	if strings.HasPrefix(state.current, "-") {
		if flag, consumed, needsValue, attachedValue, hasAttachedValue := parseCompletionFlag(state.currentFlags, state.current); consumed && (needsValue || hasAttachedValue) {
			prefix := ""
			if hasAttachedValue {
				prefix = state.current[:len(state.current)-len(attachedValue)]
			}
			valueCommand := state.currentCmd
			if valueCommand == nil {
				valueCommand = state.root
			}
			return valueCompletions(a, valueCommand, flag, state.positional, attachedValue, prefix)
		}
		return uniqueSortedPrefixStrings(appendFlagCompletionValues(nil, state.currentFlags), state.current)
	}

	if state.currentCmd == nil {
		commandSuggestions := filterSortedCompletionValues(a.rootCommandCompletionValues(), state.current)
		commandSuggestions = append(commandSuggestions, uniqueSortedPrefixStrings(append([]string(nil), state.rootBuiltins...), state.current)...)
		commandSuggestions = uniqueSortedStrings(commandSuggestions)
		if len(commandSuggestions) > 0 {
			return commandSuggestions
		}
	} else {
		commandSuggestions := make([]string, 0, visibleCommandCompletionCount(state.currentCmds))
		commandSuggestions = uniqueSortedPrefixStrings(appendCommandCompletionValues(commandSuggestions, state.currentCmds), state.current)
		if len(commandSuggestions) > 0 {
			return commandSuggestions
		}
	}

	target := state.currentCmd
	if target == nil && state.root.Runnable() {
		target = state.root
	}
	if target == nil {
		return nil
	}
	return positionalValueCompletions(a, target, target.positionalArgForIndex(len(state.positional)), state.positional, state.current)
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

func (a *App) completeDetailed(args []string) []CompletionResult {
	state := a.resolveCompletionState(args)

	if state.currentCmd == nil {
		if suggestions, handled := a.completeBuiltinDetailed(state.positional, state.current); handled {
			return suggestions
		}
	}

	if state.expectingFlag != nil {
		valueCommand := state.currentCmd
		if valueCommand == nil {
			valueCommand = state.root
		}
		return valueCompletionsDetailed(a, valueCommand, state.expectingFlag, state.positional, state.current, "", completionKindValue)
	}

	if state.current == "" {
		if state.currentCmd == nil && state.currentFlags == nil && !state.root.Runnable() {
			return a.rootCompletionResults()
		}
		suggestions := make([]CompletionResult, 0)
		if state.currentCmd == nil {
			suggestions = append(suggestions, a.rootCommandCompletionResults()...)
		} else {
			suggestions = appendCommandCompletionResults(suggestions, state.currentCmds)
		}
		if state.currentCmd == nil {
			suggestions = appendBuiltinCompletionResults(suggestions, state.rootBuiltins)
		}
		suggestions = appendFlagCompletionResults(suggestions, state.currentFlags)
		target := state.currentCmd
		if target == nil && state.root.Runnable() {
			target = state.root
		}
		if target != nil {
			suggestions = append(suggestions, positionalValueCompletionsDetailed(a, target, target.positionalArgForIndex(len(state.positional)), state.positional, "")...)
		}
		return uniqueSortedCompletionResults(suggestions, "")
	}

	if strings.HasPrefix(state.current, "-") {
		if flag, consumed, needsValue, attachedValue, hasAttachedValue := parseCompletionFlag(state.currentFlags, state.current); consumed && (needsValue || hasAttachedValue) {
			prefix := ""
			if hasAttachedValue {
				prefix = state.current[:len(state.current)-len(attachedValue)]
			}
			valueCommand := state.currentCmd
			if valueCommand == nil {
				valueCommand = state.root
			}
			return valueCompletionsDetailed(a, valueCommand, flag, state.positional, attachedValue, prefix, completionKindValue)
		}
		return uniqueSortedCompletionResults(appendFlagCompletionResults(nil, state.currentFlags), state.current)
	}

	if state.currentCmd == nil {
		commandSuggestions := filterSortedCompletionResults(a.rootCommandCompletionResults(), state.current)
		commandSuggestions = append(commandSuggestions, uniqueSortedCompletionResults(appendBuiltinCompletionResults(nil, state.rootBuiltins), state.current)...)
		commandSuggestions = uniqueSortedCompletionResults(commandSuggestions, "")
		if len(commandSuggestions) > 0 {
			return commandSuggestions
		}
	} else {
		commandSuggestions := uniqueSortedCompletionResults(appendCommandCompletionResults(nil, state.currentCmds), state.current)
		if len(commandSuggestions) > 0 {
			return commandSuggestions
		}
	}

	target := state.currentCmd
	if target == nil && state.root.Runnable() {
		target = state.root
	}
	if target == nil {
		return nil
	}
	return positionalValueCompletionsDetailed(a, target, target.positionalArgForIndex(len(state.positional)), state.positional, state.current)
}

func (a *App) resolveCompletionState(args []string) completionState {
	return a.newResolver().ResolveCompletion(args)
}

func (a *App) completeBuiltinDetailed(args []string, current string) ([]CompletionResult, bool) {
	if len(args) == 0 {
		return nil, false
	}

	var values []string
	switch args[0] {
	case "completion":
		if len(args) == 1 {
			values = filterPrefix([]string{"bash", "zsh", "fish", "powershell"}, current)
		}
		return completionResultsFromValues(values, completionKindValue), true
	case "spec":
		if len(args) == 1 {
			values = filterPrefix([]string{"json"}, current)
		}
		return completionResultsFromValues(values, completionKindValue), true
	case "docs":
		if len(args) == 1 {
			values = filterPrefix([]string{"markdown", "man"}, current)
		}
		return completionResultsFromValues(values, completionKindValue), true
	default:
		return nil, false
	}
}

func (a *App) newRootFlagSet() *FlagSet {
	root := a.rootCommand()
	return a.newRootFlagSetFor(root, io.Discard)
}

func (a *App) newRootFlagSetFor(root *Command, output io.Writer) *FlagSet {
	if a.SetFlags == nil && !a.configEnabled() && (root == nil || root.SetFlags == nil) {
		return nil
	}
	if def, ok := a.rootFlagDefinition(root); ok {
		return instantiateFlagSetFromDef(def, a.Name, output)
	}
	flagSet := NewFlagSet(a.Name, ContinueOnError)
	flagSet.SetOutput(output)
	a.configureFlagSet(flagSet, root, root)
	return flagSet
}

func (a *App) newCommandFlagSet(cmd *Command) *FlagSet {
	root := a.rootCommand()
	rootFlags := a.newRootFlagSetFor(root, io.Discard)
	return a.newCommandFlagSetFor(root, rootFlags, cmd, io.Discard)
}

func (a *App) newCommandFlagSetFor(root *Command, rootFlags *FlagSet, cmd *Command, output io.Writer) *FlagSet {
	if def, ok := a.commandFlagDefinition(root, cmd); ok {
		return instantiateFlagSetFromDef(def, cmd.Name, output)
	}
	flagSet := cloneFlagSetDefinition(rootFlags, cmd.Name, output)
	if cmd == root || cmd.SetFlags == nil {
		return flagSet
	}
	cmd.SetFlags(flagSet)
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
	return uniqueSortedStrings(appendCommandCompletionValues(nil, commands))
}

func flagCompletions(flagSet *FlagSet) []string {
	return uniqueSortedStrings(appendFlagCompletionValues(nil, flagSet))
}

func filterPrefix(values []string, prefix string) []string {
	return uniqueSortedPrefixStrings(values, prefix)
}

func valueCompletions(app *App, command *Command, flag *Flag, args []string, current string, prefix string) []string {
	return completionValues(valueCompletionsDetailed(app, command, flag, args, current, prefix, completionKindValue))
}

func valueCompletionsDetailed(app *App, command *Command, flag *Flag, args []string, current string, prefix string, kind string) []CompletionResult {
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

	values = uniqueSortedPrefixStrings(values, current)
	if prefix == "" {
		return completionResultsFromValues(values, kind)
	}

	prefixed := make([]string, 0, len(values))
	for _, value := range values {
		prefixed = append(prefixed, prefix+value)
	}
	return completionResultsFromValues(prefixed, kind)
}

func uniqueSortedStrings(values []string) []string {
	return uniqueSortedPrefixStrings(values, "")
}

func filterSortedCompletionValues(values []string, prefix string) []string {
	if len(values) == 0 {
		return nil
	}
	if prefix == "" {
		return append([]string(nil), values...)
	}
	filtered := make([]string, 0)
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func uniqueSortedPrefixStrings(values []string, prefix string) []string {
	if len(values) == 0 {
		return nil
	}

	filtered := values[:0]
	for _, value := range values {
		if value == "" {
			continue
		}
		if prefix != "" && !strings.HasPrefix(value, prefix) {
			continue
		}
		filtered = append(filtered, value)
	}
	if len(filtered) == 0 {
		return nil
	}

	slices.Sort(filtered)
	write := 1
	for read := 1; read < len(filtered); read++ {
		if filtered[read] == filtered[write-1] {
			continue
		}
		filtered[write] = filtered[read]
		write++
	}
	return filtered[:write]
}

func appendCommandCompletionValues(dst []string, commands Commands) []string {
	return appendCompletionResultValues(dst, appendCommandCompletionResults(nil, commands))
}

func appendCommandCompletionResults(dst []CompletionResult, commands Commands) []CompletionResult {
	for _, command := range commands {
		if command == nil || command.Hidden {
			continue
		}
		dst = append(dst, CompletionResult{
			Value:       command.Name,
			Description: command.Short,
			Kind:        completionKindForCommand(command),
		})
		for _, alias := range command.Aliases {
			if alias == "" {
				continue
			}
			dst = append(dst, CompletionResult{
				Value:       alias,
				Description: command.Short,
				Kind:        completionKindForCommand(command),
			})
		}
	}
	return dst
}

func appendFlagCompletionValues(dst []string, flagSet *FlagSet) []string {
	return appendCompletionResultValues(dst, appendFlagCompletionResults(nil, flagSet))
}

func appendFlagCompletionResults(dst []CompletionResult, flagSet *FlagSet) []CompletionResult {
	if flagSet == nil {
		return dst
	}
	if flagSet.def != nil {
		for _, def := range flagSet.def.Flags {
			if def.Hidden {
				continue
			}
			description := flagCompletionDescriptionFromDef(def)
			if def.Shorthand != "" {
				dst = append(dst, CompletionResult{
					Value:       "-" + def.Shorthand,
					Description: description,
					Kind:        completionKindFlag,
				})
			}
			dst = append(dst, CompletionResult{
				Value:       "--" + def.Name,
				Description: description,
				Kind:        completionKindFlag,
			})
		}
		return dst
	}
	flagSet.VisitAll(func(flag *Flag) {
		if flag.Hidden {
			return
		}
		if flag.Shorthand != "" {
			dst = append(dst, CompletionResult{
				Value:       "-" + flag.Shorthand,
				Description: flagCompletionDescription(flag),
				Kind:        completionKindForFlag(flag),
			})
		}
		dst = append(dst, CompletionResult{
			Value:       "--" + flag.Name,
			Description: flagCompletionDescription(flag),
			Kind:        completionKindForFlag(flag),
		})
	})
	return dst
}

func positionalValueCompletionsDetailed(app *App, command *Command, positional *PositionalArg, args []string, current string) []CompletionResult {
	return completionResultsFromValues(positionalValueCompletions(app, command, positional, args, current), completionKindPositional)
}

func appendBuiltinCompletionResults(dst []CompletionResult, values []string) []CompletionResult {
	for _, value := range values {
		if value == "" {
			continue
		}
		dst = append(dst, CompletionResult{
			Value: value,
			Kind:  completionKindBuiltin,
		})
	}
	return dst
}

func completionResultsFromValues(values []string, kind string) []CompletionResult {
	if len(values) == 0 {
		return nil
	}
	results := make([]CompletionResult, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		results = append(results, CompletionResult{
			Value: value,
			Kind:  kind,
		})
	}
	return results
}

func completionValues(results []CompletionResult) []string {
	if len(results) == 0 {
		return nil
	}
	values := make([]string, 0, len(results))
	for _, result := range results {
		if result.Value == "" {
			continue
		}
		values = append(values, result.Value)
	}
	return values
}

func appendCompletionResultValues(dst []string, results []CompletionResult) []string {
	for _, result := range results {
		if result.Value == "" {
			continue
		}
		dst = append(dst, result.Value)
	}
	return dst
}

func uniqueSortedCompletionResults(results []CompletionResult, prefix string) []CompletionResult {
	if len(results) == 0 {
		return nil
	}

	filtered := make([]CompletionResult, 0, len(results))
	for _, result := range results {
		if result.Value == "" {
			continue
		}
		if prefix != "" && !strings.HasPrefix(result.Value, prefix) {
			continue
		}
		filtered = append(filtered, result)
	}
	if len(filtered) == 0 {
		return nil
	}

	slices.SortFunc(filtered, func(a, b CompletionResult) int {
		return strings.Compare(a.Value, b.Value)
	})

	write := 1
	for read := 1; read < len(filtered); read++ {
		if filtered[read].Value == filtered[write-1].Value {
			continue
		}
		filtered[write] = filtered[read]
		write++
	}
	return filtered[:write]
}

func filterSortedCompletionResults(results []CompletionResult, prefix string) []CompletionResult {
	if len(results) == 0 {
		return nil
	}
	if prefix == "" {
		return append([]CompletionResult(nil), results...)
	}
	filtered := make([]CompletionResult, 0)
	for _, result := range results {
		if strings.HasPrefix(result.Value, prefix) {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

func flagCompletionDescription(flag *Flag) string {
	if flag == nil {
		return ""
	}
	_, usage := UnquoteUsage(flag)
	return usage
}

func flagCompletionDescriptionFromDef(def *flagDef) string {
	if def == nil {
		return ""
	}
	flag := &Flag{
		Name:      def.Name,
		Shorthand: def.Shorthand,
		Usage:     def.Usage,
		DefValue:  def.DefValue,
		def:       def,
	}
	_, usage := UnquoteUsage(flag)
	return usage
}

func completionKindForFlag(_ *Flag) string {
	return completionKindFlag
}

func completionKindForCommand(_ *Command) string {
	return completionKindCommand
}

func estimateStringCompletionCapacity(commands Commands, flagSet *FlagSet, includeBuiltins bool, builtinCount int) int {
	capacity := visibleCommandCompletionCount(commands) + visibleFlagCompletionCount(flagSet)
	if includeBuiltins {
		capacity += builtinCount
	}
	return capacity
}

func visibleCommandCompletionCount(commands Commands) int {
	count := 0
	for _, command := range commands {
		if command == nil || command.Hidden {
			continue
		}
		count++
		count += len(command.Aliases)
	}
	return count
}

func visibleFlagCompletionCount(flagSet *FlagSet) int {
	if flagSet == nil {
		return 0
	}
	count := 0
	for _, flag := range flagSet.formal {
		if flag == nil || flag.Hidden {
			continue
		}
		count++
		if flag.Shorthand != "" {
			count++
		}
	}
	return count
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
