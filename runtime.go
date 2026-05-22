package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
)

var errNilRuntime = errors.New("runtime is nil")
var exitProcess = os.Exit

type Runtime interface {
	Run(ctx context.Context, app *App) error
}

type CLIRuntime struct {
	Args []string
}

type REPLRuntime struct {
	In         io.Reader
	Out        io.Writer
	Err        io.Writer
	Prompt     string
	PromptFunc func(ctx context.Context, repl *REPL) string
	Welcome    string
	Driver     REPLDriver
	History    *REPLHistoryHooks
}

type AutoRuntime struct {
	Args []string
	In   io.Reader
	Out  io.Writer
	Err  io.Writer
}

func DefaultRuntime(args []string) Runtime {
	return AutoRuntime{
		Args: append([]string(nil), args...),
		In:   os.Stdin,
		Out:  os.Stdout,
		Err:  os.Stderr,
	}
}

func (a *App) DefaultRuntime(args []string) Runtime {
	return DefaultRuntime(args)
}

func (a *App) RunWith(ctx context.Context, runtime Runtime) error {
	if runtime == nil {
		return errNilRuntime
	}
	return runtime.Run(ctx, a)
}

func (a *App) RunDefault(ctx context.Context, args []string) error {
	if a == nil {
		return nil
	}
	return a.RunWith(ctx, a.DefaultRuntime(args))
}

func (a *App) RunAuto(ctx context.Context, args []string) error {
	return a.RunDefault(ctx, args)
}

func (a *App) Main(ctx context.Context, runtime Runtime) int {
	if a == nil {
		return 0
	}
	if err := a.RunWith(ctx, runtime); err != nil {
		out := a.Err
		if out == nil {
			out = os.Stderr
		}
		fmt.Fprintf(out, "Error: %v\n", err)
	}
	return a.ExitStatus()
}

func (a *App) MainDefault(ctx context.Context, args []string) int {
	if a == nil {
		return 0
	}
	return a.Main(ctx, a.DefaultRuntime(args))
}

func (a *App) MainAuto(ctx context.Context, args []string) int {
	return a.MainDefault(ctx, args)
}

func (a *App) MustRun(ctx context.Context, runtime Runtime) {
	exitProcess(a.Main(ctx, runtime))
}

func (a *App) MustRunDefault(ctx context.Context, args []string) {
	exitProcess(a.MainDefault(ctx, args))
}

func (a *App) MustRunAuto(ctx context.Context, args []string) {
	exitProcess(a.MainAuto(ctx, args))
}

func Main(app *App) {
	MainWithContext(context.Background(), app)
}

func MainWithContext(ctx context.Context, app *App) {
	if app == nil {
		return
	}
	app.MustRunAuto(ctx, os.Args[1:])
}

func (r CLIRuntime) Run(ctx context.Context, app *App) error {
	if app == nil {
		return nil
	}
	return app.runCLI(ctx, r.Args)
}

func (r REPLRuntime) Run(ctx context.Context, app *App) error {
	if app == nil {
		return nil
	}
	return app.newREPL(r.In, r.Out, r.Err, r.Prompt, r.PromptFunc, r.Welcome, r.Driver, r.History).Run(ctx)
}

func (r AutoRuntime) Run(ctx context.Context, app *App) error {
	if app == nil {
		return nil
	}
	if len(r.Args) > 0 {
		return CLIRuntime{Args: r.Args}.Run(ctx, app)
	}
	if !app.replEnabled() {
		return CLIRuntime{Args: nil}.Run(ctx, app)
	}

	in := r.In
	if in == nil {
		in = os.Stdin
	}
	out := r.Out
	if out == nil {
		out = os.Stdout
	}
	if isTTYREPL(&REPL{In: in, Out: out}) {
		return REPLRuntime{
			In:  in,
			Out: out,
			Err: r.Err,
		}.Run(ctx, app)
	}
	return CLIRuntime{Args: nil}.Run(ctx, app)
}
