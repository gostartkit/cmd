package cmd

import (
	"context"
	"io"
	"strconv"
	"testing"
	"time"
)

var (
	benchCtx         = context.Background()
	benchRunErr      error
	benchStringsSink []string
	benchResultsSink []CompletionResult
	benchSpecSink    AppSpec
	benchDocSink     string
)

func BenchmarkRunSimpleCommand(b *testing.B) {
	app := benchmarkSimpleApp()
	args := []string{"hello"}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchRunErr = app.Run(benchCtx, args)
		if benchRunErr != nil {
			b.Fatalf("run failed: %v", benchRunErr)
		}
	}
}

func BenchmarkRunNestedCommand(b *testing.B) {
	app := benchmarkNestedApp()
	args := []string{"admin", "users", "list"}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchRunErr = app.Run(benchCtx, args)
		if benchRunErr != nil {
			b.Fatalf("run failed: %v", benchRunErr)
		}
	}
}

func BenchmarkRunWideCommandTree1000(b *testing.B) {
	app := benchmarkWideApp(1000)
	args := []string{"cmd-0999"}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchRunErr = app.Run(benchCtx, args)
		if benchRunErr != nil {
			b.Fatalf("run failed: %v", benchRunErr)
		}
	}
}

func BenchmarkRunWithFlags(b *testing.B) {
	app := benchmarkFlagApp()
	args := []string{"--verbose", "--profile", "dev", "deploy", "--env", "prod", "--count", "5"}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchRunErr = app.Run(benchCtx, args)
		if benchRunErr != nil {
			b.Fatalf("run failed: %v", benchRunErr)
		}
	}
}

func BenchmarkFlagSetParse(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var verbose bool
		var count int
		var env string
		flagSet := NewFlagSet("bench", ContinueOnError)
		flagSet.SetOutput(io.Discard)
		flagSet.BoolVar(&verbose, "verbose", false, "verbose output", "v")
		flagSet.IntVar(&count, "count", 0, "item count", "c")
		flagSet.StringVar(&env, "env", "", "target env", "e")

		if err := flagSet.Parse([]string{"--verbose", "--count", "5", "--env", "prod", "arg1", "arg2"}); err != nil {
			b.Fatalf("parse failed: %v", err)
		}
	}
}

func BenchmarkFlagSetParseManyFlags(b *testing.B) {
	register := benchmarkManyFlagsRegister(32)
	cache := buildCachedFlagDefinition("bench", register)
	args := benchmarkManyFlagsArgs(32)

	b.Run("current-register-parse", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			flagSet := NewFlagSet("bench", ContinueOnError)
			flagSet.SetOutput(io.Discard)
			register(flagSet)
			if err := flagSet.Parse(args); err != nil {
				b.Fatalf("parse failed: %v", err)
			}
		}
	})

	b.Run("prototype-instantiate-parse", func(b *testing.B) {
		if !cache.Cacheable {
			b.Fatal("expected cacheable definition")
		}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			flagSet, ok := instantiateCachedFlagDefinition(cache, "bench", io.Discard)
			if !ok {
				b.Fatal("instantiate failed")
			}
			if err := flagSet.Parse(args); err != nil {
				b.Fatalf("parse failed: %v", err)
			}
		}
	})
}

func BenchmarkFlagSetCloneDefinition(b *testing.B) {
	register := benchmarkManyFlagsRegister(32)
	cache := buildCachedFlagDefinition("bench", register)

	b.Run("current-register", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			flagSet := NewFlagSet("bench", ContinueOnError)
			flagSet.SetOutput(io.Discard)
			register(flagSet)
			benchRunErr = nil
		}
	})

	b.Run("prototype-instantiate", func(b *testing.B) {
		if !cache.Cacheable {
			b.Fatal("expected cacheable definition")
		}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			flagSet, ok := instantiateCachedFlagDefinition(cache, "bench", io.Discard)
			if !ok || flagSet == nil {
				b.Fatal("instantiate failed")
			}
		}
	})
}

func BenchmarkCompleteRoot(b *testing.B) {
	app := benchmarkWideApp(1000)
	args := []string{""}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchStringsSink = app.complete(args)
	}
}

func BenchmarkCompleteDetailedRoot(b *testing.B) {
	app := benchmarkWideApp(1000)
	args := []string{""}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchResultsSink = app.completeDetailed(args)
	}
}

func BenchmarkCompleteNested(b *testing.B) {
	app := benchmarkNestedCompletionApp()
	args := []string{"admin", "users", ""}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchStringsSink = app.complete(args)
	}
}

func BenchmarkCompleteFlags(b *testing.B) {
	app := benchmarkFlagApp()
	args := []string{"deploy", "--e"}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchStringsSink = app.complete(args)
	}
}

func BenchmarkCompleteDetailedFlags(b *testing.B) {
	app := benchmarkFlagApp()
	args := []string{"deploy", "--e"}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchResultsSink = app.completeDetailed(args)
	}
}

func BenchmarkCompleteLineSimple(b *testing.B) {
	app := benchmarkFlagApp()
	line := "deploy --e"

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchStringsSink = app.CompleteLine(line, len(line))
	}
}

func BenchmarkRunLineSimple(b *testing.B) {
	app := benchmarkFlagApp()
	line := "--verbose --profile dev deploy --env prod --count 5"

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchRunErr = app.RunLine(benchCtx, line)
		if benchRunErr != nil {
			b.Fatalf("run line failed: %v", benchRunErr)
		}
	}
}

func BenchmarkFlagLookup(b *testing.B) {
	flagSet := benchmarkManyFlagsFlagSet(64)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, ok := flagSet.Lookup("string-33"); !ok {
			b.Fatal("expected lookup hit")
		}
	}
}

func BenchmarkVisitAll(b *testing.B) {
	flagSet := benchmarkManyFlagsFlagSet(64)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		flagSet.VisitAll(func(flag *Flag) {
			benchRunErr = nil
		})
	}
}

func BenchmarkSpecWideCommandTree(b *testing.B) {
	app := benchmarkWideApp(1000)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchSpecSink = app.Spec()
	}
}

func BenchmarkMarkdownDocsWideCommandTree(b *testing.B) {
	app := benchmarkWideApp(1000)
	spec := app.Spec()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchDocSink = markdownDocs(spec)
	}
}

func BenchmarkUsageWideCommandTree(b *testing.B) {
	app := benchmarkWideApp(1000)
	app.Err = io.Discard

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		app.flag = nil
		app.Usage()
	}
}

func benchmarkSimpleApp() *App {
	app := NewApp("bench")
	app.Out = io.Discard
	app.Err = io.Discard
	app.Commands = []*Command{
		{
			Name:  "hello",
			Short: "print greeting",
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		},
	}
	return app
}

func benchmarkNestedApp() *App {
	app := NewApp("bench")
	app.Out = io.Discard
	app.Err = io.Discard
	app.Commands = []*Command{
		{
			Name: "admin",
			SubCommands: []*Command{
				{
					Name: "users",
					SubCommands: []*Command{
						{
							Name: "list",
							Run: func(ctx context.Context, cmd *Command, args []string) error {
								return nil
							},
						},
					},
				},
			},
		},
	}
	return app
}

func benchmarkNestedCompletionApp() *App {
	app := benchmarkNestedApp()
	app.Commands[0].SubCommands[0].SubCommands = append(app.Commands[0].SubCommands[0].SubCommands,
		&Command{Name: "create"},
		&Command{Name: "delete"},
		&Command{Name: "describe"},
	)
	return app
}

func benchmarkWideApp(count int) *App {
	app := NewApp("bench")
	app.Out = io.Discard
	app.Err = io.Discard
	commands := make([]*Command, 0, count)
	for i := 0; i < count; i++ {
		name := "cmd-" + leftPad4(i)
		commands = append(commands, &Command{
			Name:  name,
			Short: "command " + strconv.Itoa(i),
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		})
	}
	app.Commands = commands
	return app
}

func benchmarkFlagApp() *App {
	app := NewApp("bench")
	app.Out = io.Discard
	app.Err = io.Discard

	var verbose bool
	var profile string
	var env string
	var count int

	app.SetFlags = func(f *FlagSet) {
		f.BoolVar(&verbose, "verbose", false, "verbose output", "v")
		f.StringVar(&profile, "profile", "", "profile name", "p")
	}
	app.Commands = []*Command{
		{
			Name: "deploy",
			SetFlags: func(f *FlagSet) {
				f.StringVar(&env, "env", "", "target environment", "e")
				f.IntVar(&count, "count", 0, "instance count", "c")
				f.SetEnum("env", "dev", "prod", "staging")
			},
			Run: func(ctx context.Context, cmd *Command, args []string) error {
				return nil
			},
		},
	}
	return app
}

func leftPad4(v int) string {
	switch {
	case v < 10:
		return "000" + strconv.Itoa(v)
	case v < 100:
		return "00" + strconv.Itoa(v)
	case v < 1000:
		return "0" + strconv.Itoa(v)
	default:
		return strconv.Itoa(v)
	}
}

func benchmarkManyFlagsRegister(count int) func(f *FlagSet) {
	return func(f *FlagSet) {
		for i := 0; i < count; i++ {
			switch i % 4 {
			case 0:
				var verbose bool
				f.BoolVar(&verbose, "bool-"+strconv.Itoa(i), false, "bool flag", "")
			case 1:
				var name string
				f.StringVar(&name, "string-"+strconv.Itoa(i), "default", "string flag", "")
			case 2:
				var number int
				f.IntVar(&number, "int-"+strconv.Itoa(i), 1, "int flag", "")
			default:
				var timeout time.Duration
				f.DurationVar(&timeout, "duration-"+strconv.Itoa(i), time.Second, "duration flag", "")
			}
		}
	}
}

func benchmarkManyFlagsArgs(count int) []string {
	args := make([]string, 0, count*2)
	for i := 0; i < count; i++ {
		switch i % 4 {
		case 0:
			args = append(args, "--bool-"+strconv.Itoa(i))
		case 1:
			args = append(args, "--string-"+strconv.Itoa(i), "value")
		case 2:
			args = append(args, "--int-"+strconv.Itoa(i), "7")
		default:
			args = append(args, "--duration-"+strconv.Itoa(i), "2s")
		}
	}
	return args
}

func benchmarkManyFlagsFlagSet(count int) *FlagSet {
	register := benchmarkManyFlagsRegister(count)
	cache := buildCachedFlagDefinition("bench", register)
	if cache.Cacheable {
		flagSet, ok := instantiateCachedFlagDefinition(cache, "bench", io.Discard)
		if ok {
			return flagSet
		}
	}
	flagSet := NewFlagSet("bench", ContinueOnError)
	flagSet.SetOutput(io.Discard)
	register(flagSet)
	return flagSet
}
