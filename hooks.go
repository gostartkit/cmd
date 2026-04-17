package cmd

import (
	"context"
	"time"
)

type HookContext struct {
	Context   context.Context
	App       *App
	Command   *Command
	Args      []string
	Err       error
	StartTime time.Time
}

type BeforeHook func(ctx HookContext) error
type AfterHook func(ctx HookContext)
type ErrorHook func(ctx HookContext)

func (a *App) runBeforeHooks(ctx context.Context, cmd *Command, args []string, start time.Time) error {
	hookCtx := HookContext{
		Context:   ctx,
		App:       a,
		Command:   cmd,
		Args:      append([]string(nil), args...),
		StartTime: start,
	}
	if a.BeforeRun != nil {
		if err := a.BeforeRun(hookCtx); err != nil {
			return err
		}
	}
	if cmd != nil && cmd.BeforeRun != nil {
		if err := cmd.BeforeRun(hookCtx); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) runAfterHooks(ctx context.Context, cmd *Command, args []string, start time.Time, err error) {
	hookCtx := HookContext{
		Context:   ctx,
		App:       a,
		Command:   cmd,
		Args:      append([]string(nil), args...),
		Err:       err,
		StartTime: start,
	}
	if cmd != nil && cmd.AfterRun != nil {
		cmd.AfterRun(hookCtx)
	}
	if a.AfterRun != nil {
		a.AfterRun(hookCtx)
	}
}

func (a *App) runErrorHooks(ctx context.Context, cmd *Command, args []string, start *time.Time, err error) {
	if err == nil {
		return
	}

	hookCtx := HookContext{
		Context: ctx,
		App:     a,
		Command: cmd,
		Args:    append([]string(nil), args...),
		Err:     err,
	}
	if start != nil {
		hookCtx.StartTime = *start
	}
	if cmd != nil && cmd.OnError != nil {
		cmd.OnError(hookCtx)
	}
	if a.OnError != nil {
		a.OnError(hookCtx)
	}
}

func (a *App) fail(cmd *Command, ctx context.Context, args []string, start *time.Time, err error, kind string, code int) error {
	cliErr := normalizeError(err, commandName(cmd), kind, code)
	a.setExitStatus(cliErr.ExitCode)
	a.runErrorHooks(ctx, cmd, args, start, cliErr)
	if cmd != nil && start != nil {
		a.runAfterHooks(ctx, cmd, args, *start, cliErr)
		a.emitEvent(Event{
			Type:      EventCommandFailed,
			Command:   cmd,
			Args:      args,
			Err:       cliErr,
			StartTime: *start,
			EndTime:   time.Now(),
			Duration:  time.Since(*start),
			ExitCode:  cliErr.ExitCode,
		})
	}
	return cliErr
}

func commandName(cmd *Command) string {
	if cmd == nil {
		return ""
	}
	return cmd.Name
}
