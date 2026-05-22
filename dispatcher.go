package cmd

import (
	"context"
	"fmt"
	"time"
)

type Dispatcher struct {
	App *App
}

func (a *App) newDispatcher() *Dispatcher {
	return &Dispatcher{App: a}
}

func (d *Dispatcher) Dispatch(ctx context.Context, invocation *Invocation) error {
	if d == nil || d.App == nil || invocation == nil {
		return nil
	}

	switch invocation.Kind {
	case InvocationNoop:
		return nil
	case InvocationUsage:
		d.App.Usage()
		d.App.setExitStatus(2)
		return nil
	case InvocationHelp:
		return d.dispatchHelp(invocation)
	case InvocationBuiltin:
		return d.dispatchBuiltin(ctx, invocation)
	case InvocationCommand:
		return d.dispatchCommand(ctx, invocation)
	default:
		return nil
	}
}

func (d *Dispatcher) dispatchHelp(invocation *Invocation) error {
	if len(invocation.Args) == 0 {
		d.App.Usage()
		return nil
	}

	cmd := invocation.Command
	if cmd == nil {
		return fmt.Errorf("%w, no command provided", ErrNotFound)
	}
	cmd.app = d.App
	rootFlags := d.App.newRootFlagSetFor(invocation.Root, d.App.Err)
	cmd.flag = d.App.newCommandFlagSetFor(invocation.Root, rootFlags, cmd, d.App.Err)
	cmd.Usage()
	return nil
}

func (a *App) runHelpCommand(args []string) error {
	invocation := &Invocation{
		App:    a,
		Root:   a.rootCommand(),
		Args:   append([]string(nil), args...),
		Kind:   InvocationHelp,
		IsREPL: false,
	}
	if len(args) > 0 {
		path, cmd, _, err := a.registry().ResolveCommand(args)
		if err != nil {
			return err
		}
		invocation.CommandPath = path
		invocation.Command = cmd
	}
	return a.newDispatcher().dispatchHelp(invocation)
}

func (d *Dispatcher) dispatchBuiltin(ctx context.Context, invocation *Invocation) error {
	handler, ok := invocation.Registry.Builtin(invocation.Builtin)
	if !ok {
		return d.App.fail(nil, ctx, invocation.RawArgs, nil, fmt.Errorf("%w, unknown builtin %q", ErrNotFound, invocation.Builtin), ErrorKindNotFound, 2)
	}
	if err := handler(ctx, d.App, invocation.Args); err != nil {
		return d.App.fail(nil, ctx, invocation.RawArgs, nil, err, ErrorKindInvalidArguments, 2)
	}
	return nil
}

func (d *Dispatcher) dispatchCommand(ctx context.Context, invocation *Invocation) error {
	cmd := invocation.Command
	if cmd == nil {
		return nil
	}

	cmd.app = d.App
	cmd.flag = invocation.FlagSet
	startTime := time.Now()

	if !cmd.Runnable() {
		cmd.flag.Usage()
		d.App.setExitStatus(2)
		return nil
	}

	cmd.flag.WarnDeprecated()
	if cmd.Deprecated != "" {
		fmt.Fprintf(d.App.Err, "Warning: command %q is deprecated: %s\n", cmd.Name, cmd.Deprecated)
	}

	commandArgs := append([]string(nil), invocation.Args...)
	d.App.emitEvent(Event{
		Type:      EventCommandStarted,
		Command:   cmd,
		Args:      commandArgs,
		StartTime: startTime,
	})

	if err := d.App.runBeforeHooks(ctx, cmd, commandArgs, startTime); err != nil {
		return d.App.fail(cmd, ctx, commandArgs, &startTime, err, ErrorKindRuntime, 1)
	}

	middlewareCtx := MiddlewareContext{
		Context:   ctx,
		App:       d.App,
		Command:   cmd,
		Args:      commandArgs,
		StartTime: startTime,
	}
	runFn := func(runCtx context.Context) error {
		return cmd.Run(runCtx, cmd, commandArgs)
	}
	middlewares := joinMiddlewares(d.App.Middlewares, cmd.Middlewares)
	if err := chainMiddlewares(middlewareCtx, runFn, middlewares); err != nil {
		return d.App.fail(cmd, ctx, commandArgs, &startTime, err, ErrorKindRuntime, 1)
	}

	d.App.runAfterHooks(ctx, cmd, commandArgs, startTime, nil)
	d.App.emitEvent(Event{
		Type:      EventCommandFinished,
		Command:   cmd,
		Args:      commandArgs,
		StartTime: startTime,
		EndTime:   time.Now(),
		Duration:  time.Since(startTime),
		ExitCode:  d.App.ExitStatus(),
	})
	return nil
}
