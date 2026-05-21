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

type REPL struct {
	App    *App
	Prompt string
	In     io.Reader
	Out    io.Writer
	Err    io.Writer
}

var _ LineCompleter = (*App)(nil)
var _ DetailedLineCompleter = (*App)(nil)

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

	return (&REPL{
		App: a,
		In:  in,
		Out: out,
		Err: out,
	}).Run(ctx)
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
			trimmed := strings.TrimSpace(line)

			switch trimmed {
			case "", "\n":
				continue
			case "exit", "quit", ".exit", ".quit":
				return nil
			case ".help":
				printREPLHelp(r.Out)
				continue
			}

			if err := r.App.RunLine(ctx, line); err != nil {
				fmt.Fprintf(r.Err, "Error: %v\n", err)
			}
		}
	}
}

func printREPLHelp(out io.Writer) {
	fmt.Fprintln(out, "REPL commands:")
	fmt.Fprintln(out, "  exit, quit, .exit, .quit  exit the REPL")
	fmt.Fprintln(out, "  .help                     show this help")
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
