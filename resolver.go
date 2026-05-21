package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

type InvocationKind string

const (
	InvocationNoop    InvocationKind = "noop"
	InvocationUsage   InvocationKind = "usage"
	InvocationHelp    InvocationKind = "help"
	InvocationBuiltin InvocationKind = "builtin"
	InvocationCommand InvocationKind = "command"
)

// Invocation is the shared intermediate representation between CLI/REPL input and execution.
type Invocation struct {
	App            *App
	Registry       *Registry
	Root           *Command
	CommandPath    []*Command
	Command        *Command
	RawArgs        []string
	Args           []string
	FlagSet        *FlagSet
	Positionals    []string
	InheritedFlags []parsedFlagValue
	Builtin        string
	IsREPL         bool
	Kind           InvocationKind
}

type Resolver struct {
	App      *App
	Registry *Registry
}

type resolveOptions struct {
	isREPL bool
}

func (a *App) newResolver() *Resolver {
	registry := a.registry()
	return &Resolver{
		App:      a,
		Registry: registry,
	}
}

func (a *App) runArgs(ctx context.Context, args []string, isREPL bool) error {
	if a == nil {
		return nil
	}

	a.resetRuntimeState()
	resolver := a.newResolver()
	invocation, err := resolver.Resolve(ctx, args, resolveOptions{isREPL: isREPL})
	if err != nil {
		a.currentRoot = nil
		return err
	}
	defer func() {
		a.currentRoot = nil
	}()
	return a.newDispatcher().Dispatch(ctx, invocation)
}

func (r *Resolver) Resolve(ctx context.Context, args []string, opts resolveOptions) (*Invocation, error) {
	a := r.App
	root := r.Registry.Root

	invocation := &Invocation{
		App:      a,
		Registry: r.Registry,
		Root:     root,
		RawArgs:  append([]string(nil), args...),
		IsREPL:   opts.isREPL,
	}

	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		invocation.Kind = InvocationUsage
		return invocation, nil
	}

	a.flag = nil
	a.configData = nil
	a.exitStatus = 0
	a.currentRoot = root

	if a.SetFlags != nil || a.configEnabled() || (root != nil && root.SetFlags != nil) {
		a.flag = a.newRootFlagSetFor(root, a.Err)
		a.flag.Usage = a.Usage
	}

	remainingArgs := args
	if a.flag != nil {
		if err := a.flag.ApplyEnv(); err != nil {
			return nil, a.fail(nil, ctx, nil, nil, err, ErrorKindInvalidArguments, 2)
		}
		var err error
		remainingArgs, invocation.InheritedFlags, err = a.parseAppFlags(args)
		if err != nil {
			if errors.Is(err, ErrHelp) {
				invocation.Kind = InvocationNoop
				return invocation, nil
			}
			return nil, a.fail(nil, ctx, nil, nil, err, ErrorKindInvalidArguments, 2)
		}
		if err := a.loadConfigData(); err != nil {
			return nil, a.fail(nil, ctx, nil, nil, err, ErrorKindInvalidArguments, 2)
		}
	}

	if len(remainingArgs) == 0 {
		if a.shouldRunRoot(root, remainingArgs) {
			return r.resolveCommandInvocation(ctx, root, root, args, invocation.InheritedFlags, invocation)
		}
		invocation.Kind = InvocationUsage
		return invocation, nil
	}

	if remainingArgs[0] == "help" {
		invocation.Kind = InvocationHelp
		invocation.Args = append([]string(nil), remainingArgs[1:]...)
		if len(invocation.Args) == 0 {
			return invocation, nil
		}
		path, cmd, _, err := r.Registry.ResolveCommand(invocation.Args)
		if err != nil {
			return nil, err
		}
		invocation.CommandPath = path
		invocation.Command = cmd
		return invocation, nil
	}

	if _, ok := r.Registry.Builtin(remainingArgs[0]); ok {
		invocation.Kind = InvocationBuiltin
		invocation.Builtin = remainingArgs[0]
		invocation.Args = append([]string(nil), remainingArgs[1:]...)
		return invocation, nil
	}

	name := remainingArgs[0]
	path, cmd, commandArgs, err := r.Registry.ResolveCommand(remainingArgs)
	if err != nil {
		if a.shouldRunRoot(root, remainingArgs) {
			return r.resolveCommandInvocation(ctx, root, root, args, nil, invocation)
		}

		suggestions := suggestCommand(name, root.SubCommands)
		if len(suggestions) > 0 {
			return nil, a.fail(nil, ctx, nil, nil, fmt.Errorf("%w, unknown command %q. Did you mean %s?", ErrNotFound, name, strings.Join(suggestions, " or ")), ErrorKindNotFound, 2)
		}
		return nil, a.fail(nil, ctx, nil, nil, fmt.Errorf("%w, unknown command %q", ErrNotFound, name), ErrorKindNotFound, 2)
	}

	invocation.CommandPath = path
	return r.resolveCommandInvocation(ctx, root, cmd, commandArgs, invocation.InheritedFlags, invocation)
}

func (r *Resolver) resolveCommandInvocation(ctx context.Context, root *Command, cmd *Command, args []string, inheritedFlags []parsedFlagValue, invocation *Invocation) (*Invocation, error) {
	a := r.App
	cmd.app = a

	cmd.flag = a.newCommandFlagSetFor(root, a.flag, cmd, a.Err)
	cmd.flag.Usage = func() {
		if cmd.Name == a.Name {
			a.Usage()
			return
		}
		cmd.Usage()
	}

	if err := cmd.flag.ApplyConfig(a.configData); err != nil {
		return nil, a.fail(cmd, ctx, args, nil, err, ErrorKindInvalidArguments, 2)
	}
	if err := cmd.flag.ApplyEnv(); err != nil {
		return nil, a.fail(cmd, ctx, args, nil, err, ErrorKindInvalidArguments, 2)
	}
	for _, parsedFlag := range inheritedFlags {
		if err := cmd.flag.Set(parsedFlag.Name, parsedFlag.Value); err != nil {
			return nil, a.fail(cmd, ctx, args, nil, err, ErrorKindInvalidArguments, 2)
		}
	}

	if err := cmd.flag.Parse(args); err != nil {
		if errors.Is(err, ErrHelp) {
			invocation.Kind = InvocationNoop
			return invocation, nil
		}
		return nil, a.fail(cmd, ctx, args, nil, err, ErrorKindInvalidArguments, 2)
	}
	if err := cmd.flag.Validate(); err != nil {
		return nil, a.fail(cmd, ctx, cmd.flag.Args(), nil, err, ErrorKindInvalidArguments, 2)
	}
	if err := cmd.validatePositionals(cmd.flag.Args()); err != nil {
		return nil, a.fail(cmd, ctx, cmd.flag.Args(), nil, err, ErrorKindInvalidArguments, 2)
	}

	invocation.Kind = InvocationCommand
	invocation.Command = cmd
	invocation.Args = append([]string(nil), cmd.flag.Args()...)
	invocation.Positionals = append([]string(nil), cmd.flag.Args()...)
	invocation.FlagSet = cmd.flag
	return invocation, nil
}

func (r *Resolver) ResolveCompletion(args []string) completionState {
	root := r.Registry.Root
	current := ""
	completed := args
	if len(args) > 0 {
		current = args[len(args)-1]
		completed = args[:len(args)-1]
	}

	rootFlags := r.App.newRootFlagSetFor(root, io.Discard)
	currentFlags := rootFlags
	currentCommands := root.SubCommands
	var currentCommand *Command
	var currentOwner *Command
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

		cmd := r.Registry.Lookup(currentOwner, token)
		if cmd == nil {
			positionalArgs = append(positionalArgs, token)
			continue
		}
		currentCommand = cmd
		currentOwner = cmd
		currentFlags = r.App.newCommandFlagSetFor(root, rootFlags, cmd, io.Discard)
		currentCommands = cmd.SubCommands
	}

	return completionState{
		root:          root,
		rootBuiltins:  r.Registry.VisibleBuiltins(),
		current:       current,
		currentFlags:  currentFlags,
		currentCmds:   currentCommands,
		currentCmd:    currentCommand,
		expectingFlag: expectingValue,
		positional:    positionalArgs,
	}
}
