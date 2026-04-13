package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestAppRun(t *testing.T) {
	var runCount int
	var capturedArgs []string

	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name: "foo",
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				runCount++
				capturedArgs = args
				return nil
			},
		},
	}

	err := app.Run(context.Background(), []string{"foo", "arg1", "arg2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runCount != 1 {
		t.Errorf("expected runCount 1, got %d", runCount)
	}

	if len(capturedArgs) != 2 || capturedArgs[0] != "arg1" || capturedArgs[1] != "arg2" {
		t.Errorf("unexpected args: %v", capturedArgs)
	}
}

func TestAppSubCommand(t *testing.T) {
	var subRunCount int

	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name: "foo",
			SubCommands: []*Command{
				{
					Name: "bar",
					Run: func(ctx context.Context, cmd *Command, args []string) error {
						subRunCount++
						return nil
					},
				},
			},
		},
	}

	err := app.Run(context.Background(), []string{"foo", "bar"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if subRunCount != 1 {
		t.Errorf("expected subRunCount 1, got %d", subRunCount)
	}
}

func TestAppFlags(t *testing.T) {
	app := NewApp("test")
	var verbose bool
	var count int

	app.Commands = []*Command{
		{
			Name: "foo",
			SetFlags: func(f *FlagSet) {
				f.BoolVar(&verbose, "verbose", false, "v", "v")
				f.IntVar(&count, "count", 0, "c", "c")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		},
	}

	err := app.Run(context.Background(), []string{"foo", "-v", "--count=10"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !verbose {
		t.Errorf("expected verbose true")
	}

	if count != 10 {
		t.Errorf("expected count 10, got %d", count)
	}
}

func TestAppSuggestions(t *testing.T) {
	app := NewApp("test")
	app.Commands = []*Command{
		{Name: "status"},
		{Name: "commit"},
	}

	err := app.Run(context.Background(), []string{"statu"})
	if err == nil {
		t.Fatal("expected error for unknown command")
	}

	if !strings.Contains(err.Error(), "Did you mean status?") {
		t.Errorf("expected suggestion in error, got: %v", err)
	}
}

func TestAppContextCancellation(t *testing.T) {
	app := NewApp("test")
	app.Commands = []*Command{
		{
			Name: "long",
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := app.Run(ctx, []string{"long"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
