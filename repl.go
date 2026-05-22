package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

type LineCompleter interface {
	CompleteLine(line string, cursor int) []string
}

type REPLDriver interface {
	Run(ctx context.Context, repl *REPL) error
}

type REPLConfig struct {
	Enabled bool
	Prompt  string
	Welcome string
	Driver  REPLDriver
}

type REPL struct {
	App     *App
	Prompt  string
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
	Welcome string
	Driver  REPLDriver
}

type BasicREPLDriver struct{}

var _ LineCompleter = (*App)(nil)
var _ DetailedLineCompleter = (*App)(nil)

func (a *App) EnableREPL() {
	if a == nil {
		return
	}
	a.REPL.Enabled = true
}

func (a *App) ConfigureREPL(fn func(cfg *REPLConfig)) {
	if a == nil || fn == nil {
		return
	}
	fn(&a.REPL)
}

func (a *App) replEnabled() bool {
	return a != nil && a.REPL.Enabled
}

func (a *App) RunLine(ctx context.Context, line string) error {
	if a == nil {
		return nil
	}

	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	args, err := LexLine(line)
	if err != nil {
		return err
	}
	return a.runArgs(ctx, args, true)
}

func (a *App) CompleteLine(line string, cursor int) []string {
	if a == nil {
		return nil
	}

	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(line) {
		cursor = len(line)
	}

	args, current, _ := LexLineForCompletion(line[:cursor])
	return a.complete(append(args, current))
}

func (a *App) CompleteLineDetailed(line string, cursor int) []CompletionResult {
	if a == nil {
		return nil
	}

	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(line) {
		cursor = len(line)
	}

	args, current, _ := LexLineForCompletion(line[:cursor])
	return a.completeDetailed(append(args, current))
}

func (a *App) RunREPL(ctx context.Context, in io.Reader, out io.Writer) error {
	if a == nil {
		return nil
	}
	return a.RunWith(ctx, REPLRuntime{In: in, Out: out, Err: out})
}

func (a *App) runREPLBuiltin(ctx context.Context, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: %s repl", a.Name)
	}
	return a.RunWith(ctx, REPLRuntime{In: os.Stdin, Out: os.Stdout, Err: os.Stdout})
}

func (a *App) newREPL(in io.Reader, out io.Writer, errOut io.Writer, prompt string, welcome string, driver REPLDriver) *REPL {
	if a == nil {
		return nil
	}

	if prompt == "" {
		prompt = a.REPL.Prompt
	}
	if welcome == "" {
		welcome = a.REPL.Welcome
	}
	if driver == nil {
		driver = a.REPL.Driver
	}
	if errOut == nil && out != nil {
		errOut = out
	}

	return &REPL{
		App:     a,
		Prompt:  prompt,
		Welcome: welcome,
		Driver:  driver,
		In:      in,
		Out:     out,
		Err:     errOut,
	}
}

func (r *REPL) Run(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if r.App == nil {
		return errors.New("repl app is nil")
	}

	if r.Prompt == "" {
		r.Prompt = "> "
	}
	if r.In == nil {
		r.In = os.Stdin
	}
	if r.Out == nil {
		r.Out = os.Stdout
	}
	if r.Err == nil {
		r.Err = os.Stderr
	}

	prevOut, prevErr := r.App.Out, r.App.Err
	r.App.Out = r.Out
	r.App.Err = r.Err
	defer func() {
		r.App.Out = prevOut
		r.App.Err = prevErr
	}()

	if r.Welcome != "" {
		fmt.Fprintln(r.Out, strings.TrimRight(r.Welcome, "\r\n"))
	}

	driver := r.Driver
	if driver == nil {
		driver = defaultREPLDriver(r)
	}
	return driver.Run(ctx, r)
}

func (d BasicREPLDriver) Run(ctx context.Context, r *REPL) error {
	type lineResult struct {
		line string
		err  error
	}

	lines := make(chan lineResult, 2)
	go func() {
		reader := bufio.NewReader(r.In)
		for {
			line, err := reader.ReadString('\n')
			if len(line) > 0 {
				select {
				case lines <- lineResult{line: line}:
				case <-ctx.Done():
					close(lines)
					return
				}
			}
			if err != nil {
				select {
				case lines <- lineResult{err: err}:
				case <-ctx.Done():
				}
				close(lines)
				return
			}
		}
	}()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		fmt.Fprint(r.Out, r.Prompt)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case result, ok := <-lines:
			if !ok {
				return nil
			}

			if result.err != nil {
				if errors.Is(result.err, io.EOF) {
					return nil
				}
				return result.err
			}

			line := strings.TrimRight(result.line, "\r\n")
			exit, err := r.handleLine(ctx, line)
			if exit {
				return nil
			}
			if err != nil {
				fmt.Fprintf(r.Err, "Error: %v\n", err)
			}
		}
	}
}

func (r *REPL) handleLine(ctx context.Context, line string) (bool, error) {
	trimmed := strings.TrimSpace(line)
	switch trimmed {
	case "", "\n":
		return false, nil
	case "exit", "quit", ".exit", ".quit":
		return true, nil
	case ".help":
		printREPLHelp(r.Out)
		return false, nil
	}

	return false, r.App.RunLine(ctx, line)
}

func defaultREPLDriver(r *REPL) REPLDriver {
	if isTTYREPL(r) {
		return TerminalREPLDriver{}
	}
	return BasicREPLDriver{}
}

func printREPLHelp(out io.Writer) {
	fmt.Fprintln(out, "REPL commands:")
	fmt.Fprintln(out, "  exit, quit, .exit, .quit  exit the REPL")
	fmt.Fprintln(out, "  .help                     show this help")
	fmt.Fprintln(out, "  <Tab>                     complete commands, flags, and values")
}

func (a *App) resetRuntimeState() {
	if a == nil {
		return
	}

	a.mu.Lock()
	a.exitStatus = 0
	a.mu.Unlock()
	a.flag = nil
	a.configData = nil
	a.currentRoot = nil
}
