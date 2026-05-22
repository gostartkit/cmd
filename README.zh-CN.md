# cmd

[English](./README.md) | 简体中文

`cmd` 是一个面向 Go 的现代命令行库。它保留了轻量的 API 形态，同时补齐了产品级 CLI 常用能力：

- 子命令与递归命令树
- 全局参数与命令参数
- 位置参数 schema
- `env / config / CLI / default` 多来源绑定
- shell completion
- REPL 与 cursor-aware line completion API
- 机器可读 `spec`
- Markdown / man 文档生成
- hooks / middleware / observer
- 统一错误类型与退出码

这份文档按“从快速开始到平台化集成”的顺序组织，覆盖当前库的主要使用方式。

## 目录

1. [安装](#安装)
2. [快速开始](#快速开始)
3. [CLI + REPL 教程](#cli--repl-教程)
4. [两种使用模式](#两种使用模式)
5. [命令模型](#命令模型)
6. [参数模型](#参数模型)
7. [解析规则](#解析规则)
8. [帮助与内建命令](#帮助与内建命令)
9. [配置、环境变量与优先级](#配置环境变量与优先级)
10. [位置参数](#位置参数)
11. [completion](#completion)
12. [REPL 与行执行](#repl-与行执行)
13. [机器可读 spec](#机器可读-spec)
14. [文档生成 docs](#文档生成-docs)
15. [生命周期 hooks](#生命周期-hooks)
16. [middleware](#middleware)
17. [observer 与 telemetry](#observer-与-telemetry)
18. [统一错误与退出码](#统一错误与退出码)
19. [自定义扩展 metadata](#自定义扩展-metadata)
20. [常见模式](#常见模式)
21. [API 速查](#api-速查)

## 安装

按你的项目实际 module 路径引入。例如当前仓库中的包路径是：

```go
import "pkg.gostartkit.com/cmd"
```

## 快速开始

下面是一个最小可运行示例，包含：

- 一个全局参数 `--verbose`
- 一个子命令 `version`
- 一个子命令 `hello`
- 命令级 flag、位置参数、env 绑定

```go
package main

import (
	"context"
	"fmt"

	"pkg.gostartkit.com/cmd"
)

var (
	verbose bool
	name    string
	version = "v1.0.0"
)

func main() {
	cmd.SetFlags(func(f *cmd.FlagSet) {
		f.BoolVar(&verbose, "verbose", false, "enable verbose output", "v")
		f.SetCategory("verbose", "Global")
	})

	cmd.AddCommands(
		&cmd.Command{
			Name:      "version",
			UsageLine: "app version",
			Short:     "print version",
			Run: func(ctx context.Context, c *cmd.Command, args []string) error {
				fmt.Println(version)
				return nil
			},
		},
		&cmd.Command{
			Name:      "hello",
			UsageLine: "app hello [flags] <target>",
			Short:     "print greeting",
			Examples: []string{
				"app hello team --name sam",
				"APP_NAME=sam app hello user",
			},
			Positionals: []cmd.PositionalArg{
				{Name: "target", Usage: "greeting target", Required: true, Enum: []string{"team", "user"}},
			},
			SetFlags: func(f *cmd.FlagSet) {
				f.StringVar(&name, "name", "", "name to greet", "n")
				f.BindEnv("name", "APP_NAME")
				f.MarkRequired("name")
				f.SetEnum("name", "sam", "sara", "tom")
			},
			Run: func(ctx context.Context, c *cmd.Command, args []string) error {
				if verbose {
					fmt.Printf("[verbose] target=%s name=%s\n", args[0], name)
				}
				fmt.Printf("hello %s (%s)\n", name, args[0])
				return nil
			},
		},
	)

	cmd.Execute()
}
```

可以直接使用这些命令：

```bash
app version
app --verbose hello team --name sam
APP_NAME=sara app hello user
app hello team -n tom
```

## CLI + REPL 教程

如果你的目标是“定义一次命令树，同时支持普通 CLI 和带智能提示的 REPL”，推荐按下面这个流程接入。

### 1. 定义一份共享命令树

建议使用显式 `App`，这样 CLI、REPL、测试和嵌入式运行时都能复用同一个实例：

```go
package main

import (
	"context"
	"fmt"

	"pkg.gostartkit.com/cmd"
)

func buildApp() *cmd.App {
	app := cmd.NewApp("ops")
	app.Short = "Operations console"

	var verbose bool
	app.ConfigureFlags(func(f *cmd.FlagSet) {
		f.BoolVar(&verbose, "verbose", false, "verbose output", "v")
	})

	app.AddCommands(
		&cmd.Command{
			Name:      "deploy",
			UsageLine: "ops deploy [flags] <env>",
			Short:     "deploy service",
			Positionals: []cmd.PositionalArg{
				{
					Name:       "env",
					Required:   true,
					Enum:       []string{"dev", "staging", "prod"},
					Completion: func(ctx cmd.CompletionContext) []string { return []string{"dev", "staging", "prod"} },
				},
			},
			SetFlags: func(f *cmd.FlagSet) {
				var region string
				f.StringVar(&region, "region", "", "target region", "r")
				f.SetCompletion("region", func(ctx cmd.CompletionContext) []string {
					return []string{"cn", "us", "eu"}
				})
			},
			Run: func(ctx context.Context, c *cmd.Command, args []string) error {
				if verbose {
					fmt.Printf("[verbose] deploy to %s\n", args[0])
				}
				fmt.Printf("deploy %s\n", args[0])
				return nil
			},
		},
	)

	return app
}
```

关键点是：

- 子命令、flag、位置参数只定义一次
- `Enum`、`SetCompletion(...)`、`PositionalArg.Completion` 这些补全元数据也跟着命令树一起定义
- CLI 和 REPL 都复用同一套 completion engine

### 2. 开启内建 REPL 入口

如果你希望用户通过 `app repl` 进入交互模式，直接启用内建 REPL：

```go
app.EnableREPL()
```

也可以顺手配置 prompt / welcome：

```go
app.ConfigureREPL(func(cfg *cmd.REPLConfig) {
	cfg.Prompt = "ops> "
	cfg.Welcome = "type .help or press Tab"
})
```

### 3. 用统一入口启动

最省样板的主程序写法是：

```go
func main() {
	app := buildApp()
	app.EnableREPL()
	cmd.Main(app)
}
```

这会自动按默认策略选择运行模式：

- `ops deploy prod` 走普通 CLI
- `ops repl` 进入 REPL

如果你想在代码里显式选择，也可以：

```go
err := app.RunWith(ctx, cmd.CLIRuntime{Args: []string{"deploy", "prod"}})
err = app.RunWith(ctx, cmd.REPLRuntime{In: os.Stdin, Out: os.Stdout})
```

### 4. CLI 下怎么用

命令行为普通 CLI：

```bash
ops deploy prod
ops deploy prod --region us
ops --verbose deploy dev
```

### 5. REPL 下怎么用

进入 REPL：

```bash
ops repl
```

在 TTY 终端里，默认 REPL driver 会自动支持：

- `Tab`：补全命令、flag、位置参数和值
- 连续按 `Tab`：翻页查看更多候选
- 输入过程中实时 hint：展示当前上下文的推荐项
- inline ghost text：灰色显示当前最佳补全后缀
- `↑ / ↓`：翻历史命令
- `← / →`：左右移动光标
- `Backspace / Delete`：删除字符

例如：

```text
ops> dep
hint: deploy - deploy service

ops> deploy --r
hint: --region - target region

ops> deploy p
hint: prod
```

### 6. 什么信息会自动带到 REPL

只要这些信息定义在命令树上，REPL 就会自动继承：

- 子命令名和 alias
- 全局 flag 与命令 flag
- flag 的 `Usage` 文案
- 位置参数定义
- `Enum`
- `SetCompletion(...)`
- `PositionalArg.Completion`
- `Short` 描述

这也是推荐把“补全规则”定义在 `Command` / `FlagSet` / `PositionalArg` 上，而不是单独给 REPL 写一套逻辑的原因。

### 7. 推荐接入方式

对大多数应用，推荐这套最小组合：

1. `buildApp()` 里只维护一份命令树
2. 用 `Enum` 和 `SetCompletion(...)` 把值补全定义在命令模型上
3. `app.EnableREPL()`
4. `cmd.Main(app)`

这样通常就已经能同时拿到：

- 普通 CLI
- shell completion
- REPL
- REPL 智能提示
- REPL 命令 / 参数补全
- REPL 历史与行内编辑

## 两种使用模式

这个库支持两种风格。

### 1. 全局默认实例 `DefaultApp`

适合单二进制、主程序入口直接使用：

```go
cmd.SetFlags(...)
cmd.AddCommands(...)
cmd.Execute()
```

对应的全局入口有：

- `SetFlags`
- `AddCommands`
- `SetUsageTemplate`
- `Execute`

这些入口只是共享 `DefaultApp` 实例上的一层薄包装，真实执行模型仍然是 `App + Root Command`。

### 2. 显式实例 `App`

适合：

- 多个 CLI 实例并存
- 测试
- 嵌入式调用
- 框架/平台层封装

```go
app := cmd.NewApp("myapp")
app.SetFlags = func(f *cmd.FlagSet) { ... }
app.Commands = []*cmd.Command{...}

err := app.Run(context.Background(), []string{"hello", "team"})
if err != nil {
	// handle
}
```

如果你希望继续使用显式实例，但不直接改字段，也可以用实例侧的薄包装方法：

```go
app := cmd.NewApp("myapp")
app.ConfigureFlags(func(f *cmd.FlagSet) { ... })
app.SetRootCommand(&cmd.Command{ ... })
app.AddCommands(...)

err := app.Execute([]string{"hello", "team"})
```

## 命令模型

核心类型是 `App` 和 `Command`。

### App

`App` 表示整个 CLI 应用，常用字段包括：

- `Name`: 应用名
- `Short`: 简短说明
- `Long`: 长说明
- `Root`: 可选的根命令
- `Commands`: 顶层命令列表
- `SetFlags`: 配置全局 flag
- `BeforeRun / AfterRun / OnError`: 生命周期 hook
- `Middlewares`: 中间件
- `Observers`: 事件观察者
- `Extensions`: 自定义元数据

### Root Command

现在 `App` 也可以通过 `App.Root` 拥有一个真正的根命令。

- `App.SetFlags` 仍然是 app 级全局 flag 入口。
- `App.Commands` 仍然兼容，内部会被视为 root subcommands。
- `App.Root.SetFlags` 会合并进 root/global flag 集，并对子命令可见。
- 如果 root command 定义了 `Run`，只执行二进制本身时会运行 root command。
- 如果 root command 没有 `Run` 但有子命令，只执行二进制本身时会输出 usage。

```go
app := cmd.NewApp("myapp")

var (
	verbose bool
	profile string
)

app.SetFlags = func(f *cmd.FlagSet) {
	f.BoolVar(&verbose, "verbose", false, "enable verbose output", "v")
}

app.Root = &cmd.Command{
	UsageLine: "myapp [flags] [target]",
	Short:     "root entrypoint",
	Examples:  []string{"myapp team", "myapp version"},
	Positionals: []cmd.PositionalArg{
		{Name: "target", Usage: "target name"},
	},
	SetFlags: func(f *cmd.FlagSet) {
		f.StringVar(&profile, "profile", "", "profile name", "p")
	},
	Run: func(ctx context.Context, c *cmd.Command, args []string) error {
		fmt.Printf("root args=%v verbose=%v profile=%s\n", args, verbose, profile)
		return nil
	},
	SubCommands: []*cmd.Command{
		{
			Name:      "version",
			UsageLine: "myapp version",
			Short:     "print version",
			Run: func(ctx context.Context, c *cmd.Command, args []string) error {
				fmt.Println("v1.0.0")
				return nil
			},
		},
	},
}
```

### Command

`Command` 表示一个命令节点，支持子命令树。常用字段包括：

- `Name`
- `Aliases`
- `UsageLine`
- `Short`
- `Long`
- `Examples`
- `Positionals`
- `SetFlags`
- `Run`
- `SubCommands`
- `Deprecated`
- `Hidden`
- `BeforeRun / AfterRun / OnError`
- `Middlewares`
- `Observers`
- `Extensions`

### 定义子命令

```go
cmdAdmin := &cmd.Command{
	Name:      "admin",
	UsageLine: "app admin",
	Short:     "admin operations",
	SubCommands: []*cmd.Command{
		{
			Name:      "users",
			UsageLine: "app admin users",
			Short:     "manage users",
			Run: func(ctx context.Context, c *cmd.Command, args []string) error {
				return nil
			},
		},
	},
}
```

## 参数模型

参数由 `FlagSet` 管理。这个库既支持：

- 全局参数
- 命令级参数
- 位置参数

### 定义 flag

```go
var (
	force  bool
	count  int
	format string
)

f.BoolVar(&force, "force", false, "force operation", "f")
f.IntVar(&count, "count", 1, "retry count", "c")
f.StringVar(&format, "format", "text", "output format", "")
```

支持的类型包括：

- `BoolVar`
- `IntVar`
- `Int64Var`
- `UintVar`
- `Uint64Var`
- `StringVar`
- `Float64Var`
- `DurationVar`
- `TextVar`
- `Func`
- `BoolFunc`

也提供全局版同名函数，作用于默认 `CommandLine`。

### flag 元数据

你可以在定义后继续声明元数据：

```go
f.StringVar(&format, "format", "text", "output format", "")
f.BindEnv("format", "APP_FORMAT")
f.BindConfig("format", "output.format")
f.SetEnum("format", "json", "yaml", "text")
f.MarkRequired("format")
f.MarkHidden("format")
f.MarkDeprecated("format", "use --output instead")
f.SetCategory("format", "Output")
f.SetExample("format", "json")
```

这些元数据会同时影响：

- 解析与校验
- 帮助输出
- completion
- `spec`
- `docs`

## 解析规则

当前解析行为是库的一个重要特性。

### 1. 支持全局 flag 在命令前

```bash
app --verbose version
app --config app.json hello
```

### 2. 支持命令参数和位置参数交错

```bash
app hello team -n sam
app hello team --name sam
app hello team extra --name sam
```

也就是说，命令参数不要求必须全部写在位置参数前面。

### 3. `--` 之后停止解析 flag

```bash
app hello -- --name-not-a-flag
```

### 4. `help` 会按命令上下文路由

```bash
app help hello
app --verbose help hello
app hello --help
```

### 5. 拼写建议

对于未知命令和未知参数，库会提供建议，例如：

- `statu` -> `status`
- `--verboes` -> `--verbose`

## 帮助与内建命令

内建命令包括：

- `help`
- `completion`
- `spec`
- `docs`

如果你自己定义了同名命令，则用户命令优先，内建命令会让位。

### 自定义 Usage 模板

如果你使用默认全局实例，可以改默认 usage 模板：

```go
cmd.SetUsageTemplate(`
{{.Name}} - {{.Short}}

Usage:
  {{.Name}} [flags] <command>
`)
```

为了追求更低开销，usage 渲染使用库内置的轻量替换器，不再依赖 `text/template`。自定义模板支持普通文本以及 `{{.Name}}`、`{{.Short}}`、`{{.Long}}`、`{{.UsageLine}}` 等简单字段。

## 配置、环境变量与优先级

### 开启 JSON 配置支持

```go
app := cmd.NewApp("app")
app.EnableConfigSupport()
```

启用后，库会自动注入内建全局参数：

```bash
app --config app.json hello
```

默认配置文件加载器是 JSON：

```json
{
  "name": "from-config",
  "output": {
    "format": "json"
  }
}
```

### 绑定 env

```go
f.StringVar(&name, "name", "", "target name", "n")
f.BindEnv("name", "APP_NAME", "LEGACY_NAME")
```

### 绑定 config key

```go
f.StringVar(&format, "format", "", "output format", "")
f.BindConfig("format", "output.format")
```

### 优先级

当前优先级固定为：

`CLI flag > env > config > default`

例如：

```bash
app --config app.json hello
APP_NAME=sam app --config app.json hello
app --config app.json hello --name cli
```

### 自定义配置入口

你也可以覆盖：

- `ConfigLoader`
- `ConfigFlag`

例如替换成自己的加载逻辑。

## 位置参数

位置参数由 `Command.Positionals` 描述。

### 基本用法

```go
cmdDeploy := &cmd.Command{
	Name:      "deploy",
	UsageLine: "app deploy <env> [service]",
	Positionals: []cmd.PositionalArg{
		{Name: "env", Usage: "target environment", Required: true, Enum: []string{"dev", "staging", "prod"}},
		{Name: "service", Usage: "service name"},
	},
	Run: func(ctx context.Context, c *cmd.Command, args []string) error {
		env := args[0]
		service := ""
		if len(args) > 1 {
			service = args[1]
		}
		_ = env
		_ = service
		return nil
	},
}
```

### variadic 参数

```go
Positionals: []cmd.PositionalArg{
	{Name: "files", Variadic: true, Usage: "input files"},
}
```

### completion

位置参数也支持：

- `Enum`
- `Completion`
- `Extensions`

```go
Positionals: []cmd.PositionalArg{
	{
		Name: "service",
		Completion: func(ctx cmd.CompletionContext) []string {
			return []string{"api", "worker", "web"}
		},
	},
}
```

### 校验

库会自动处理：

- 缺少必填位置参数
- 非 variadic 命令的多余参数
- enum 值校验

## completion

### 为 shell 生成脚本

```bash
app completion bash > /etc/bash_completion.d/app
app completion zsh > "${fpath[1]}/_app"
app completion fish > ~/.config/fish/completions/app.fish
app completion powershell > app.ps1
```

### completion 支持的内容

- 命令名
- 命令别名
- 全局 flag
- 命令 flag
- flag enum 值
- flag 动态 completion
- 位置参数 enum 值
- 位置参数动态 completion
- 内建命令及其参数

例如：

```go
f.StringVar(&format, "format", "", "output format", "f")
f.SetEnum("format", "json", "yaml", "text")

f.StringVar(&name, "name", "", "target name", "n")
f.SetCompletion("name", func(ctx cmd.CompletionContext) []string {
	return []string{"sam", "sara", "tom"}
})
```

内建命令本身也有补全：

- `app completion <shell>`
- `app spec json`
- `app docs markdown`
- `app docs man`

### 程序化 completion

readline、TUI、编辑器或 agent 集成可以直接使用 line completion API。它们复用同一套命令树和 completion engine，不需要重新定义命令关系。

```go
plain := app.CompleteLine("deploy --e", len("deploy --e"))
detailed := app.CompleteLineDetailed("deploy --e", len("deploy --e"))
```

`CompleteLine` 返回纯字符串，保持旧集成兼容。`CompleteLineDetailed` 返回带 metadata 的结果，适合更丰富的 UI：

```go
type CompletionResult struct {
	Value       string
	Description string
	Kind        string
}
```

当前 `Kind` 取值包括：

- `command`
- `flag`
- `value`
- `positional`
- `builtin`

shell completion 仍然通过 `__complete` 输出纯文本，不会输出结构化 metadata。

## REPL 与行执行

REPL API 可以让嵌入式程序复用现有 `App`、命令树、flags、位置参数和 completion 逻辑，不需要重新拼装 dispatch。

```go
err := app.RunLine(ctx, `deploy "hello world" --env prod`)
```

`RunLine` 会忽略空行，把 shell-like 输入拆成 args，然后调用 `App.Run(ctx, args)`。拆分器支持空格、单引号、双引号和反斜杠转义。

启动交互循环：

```go
err := app.RunREPL(ctx, os.Stdin, os.Stdout)
```

如果你希望显式选择 runtime，可以使用统一的 runtime interface：

```go
err := app.RunWith(ctx, cmd.CLIRuntime{Args: os.Args[1:]})
err = app.RunWith(ctx, cmd.REPLRuntime{In: os.Stdin, Out: os.Stdout})
err = app.RunDefault(ctx, os.Args[1:])
```

对于应用入口，也可以使用更贴近 main 的 helper：

```go
app.RunAuto(ctx, os.Args[1:])
app.MustRunDefault(ctx, os.Args[1:])
cmd.Main(app)
```

如果你希望同一个二进制直接暴露 REPL 模式，而不是自己额外定义命令，可以开启内建 REPL 入口：

```go
app.EnableREPL()
```

之后用户可以这样进入 REPL：

```bash
app repl
```

也可以直接配置 REPL runtime：

```go
repl := &cmd.REPL{
	App:    app,
	Prompt: "app> ",
	In:     in,
	Out:    out,
	Err:    errOut,
}
err := repl.Run(ctx)
```

如果你需要动态 prompt，可以配置 `PromptFunc`。它会在每次渲染前重新计算：

```go
app.ConfigureREPL(func(cfg *cmd.REPLConfig) {
	cfg.Prompt = "app> "
	cfg.PromptFunc = func(ctx context.Context, repl *cmd.REPL) string {
		if repl.App == nil {
			return ""
		}
		return repl.App.Name + "> "
	}
})
```

如果 `PromptFunc` 返回空字符串，REPL 会回退到 `Prompt`，再回退到默认 prompt `"> "`。

也可以通过 hooks 加载和持久化历史：

```go
app.ConfigureREPL(func(cfg *cmd.REPLConfig) {
	cfg.History = &cmd.REPLHistoryHooks{
		Load: func(ctx context.Context) ([]string, error) {
			return []string{"deploy prod", "status"}, nil
		},
		Append: func(ctx context.Context, line string) error {
			fmt.Println("persist history:", line)
			return nil
		},
	}
})
```

`Load` 会在 REPL 启动时调用。`Append` 会在一条非空输入被接受并准备执行时调用。当前会话内的内存历史仍然保留，hooks 则用于接入外部持久化。

REPL 内建命令包括：

- `exit`
- `quit`
- `.exit`
- `.quit`
- `.help`

当 stdin/stdout 连接到 TTY 时，默认 REPL driver 还会启用行内编辑、历史记录导航、实时的 context-aware hint、当前最佳补全的 inline ghost text，以及基于同一套命令树和 value completion hook 的 `Tab` 补全。候选列表也会按 kind 标记，方便区分 command、flag、value 和 positional argument；候选很多时，连续按 `Tab` 可以翻页查看。

在行编辑阶段，terminal REPL 会让 stdin 处于 raw mode。用户按回车提交命令后，driver 会先临时恢复正常终端模式，再执行命令；如果 REPL 继续运行，则在命令执行完成后重新进入 raw mode 并重绘交互界面。这样命令处理函数内部如果需要继续读取 stdin、做确认、读密码、或执行更传统的终端交互，也不会和 REPL 行编辑互相冲突。

单条命令失败时 REPL 会打印错误并继续运行。`context.Canceled` 或输入 EOF 会退出循环。

## 机器可读 spec

### 输出

```bash
app spec
app spec json
```

### `spec` 包含的内容

`spec` 是当前命令树的版本化契约，适合：

- 静态站点生成
- IDE 集成
- 控制台 UI
- agent / AI 工具链
- 自动化测试

当前导出包括：

- `schema_version`
- `surface`
- `available_surfaces`
- `builtins`
- `capabilities`
- `config`
- app / command hooks 信息
- middleware / observer 标记
- global flags
- command tree
- stable command ID / handler ID
- positionals
- flags
- `extensions`

### surface-aware 导出

`Spec()` 保留默认 / base 契约。如果你需要从同一套命令树导出面向 REPL/runtime 的 schema，可以导出指定 surface：

```go
cliSpec := app.Spec()
replSpec := app.SpecFor(cmd.SurfaceREPL)
```

这适合 CLI 和 REPL 只在 usage line、positionals 必填规则等方面存在差异的场景。

### `CommandSpec` / `FlagSpec` / `PositionalSpec` 中的关键信息

- `id`
- `handler_id`
- `path`
- `kind`
- `enum`
- `required`
- `repeatable`
- `deprecated`
- `completion_key`
- `supports_completion`
- `source_order`
- `extensions`

### 示例

```bash
app spec json > spec.json
```

示例字段：

```json
{
  "schema_version": "v2",
  "name": "app",
  "surface": "repl",
  "builtins": ["help", "completion", "spec", "docs"],
  "capabilities": {
    "completion_keys": true,
    "docs_export": true,
    "middleware": true,
    "observers": true,
    "surface_overrides": true,
    "stable_ids": true
  }
}
```

## 文档生成 docs

`docs` 基于 `Spec()` 生成文档，因此和默认 `spec`、completion、帮助输出共享同一份命令模型。如果你需要 REPL/runtime 专用 schema，请额外用 `SpecFor(surface)` 导出。

### 单页输出

```bash
app docs markdown
app docs man
```

### 多文件导出

```bash
app docs markdown ./docs
app docs man ./manpages
```

### 导出结果

#### Markdown bundle

- `README.md`
- `commands/<command>.md`
- `commands/<command>/<subcommand>.md`

#### man bundle

- `<app>.1`
- `<app>-<command>.1`
- `<app>-<command>-<subcommand>.1`

### Markdown frontmatter

Markdown 文档会自动带 frontmatter，适合：

- Hugo / Docusaurus / Astro / MkDocs 这类站点工具
- 搜索索引器
- 内容流水线

frontmatter 中会写入：

- `kind`
- `title`
- `summary`
- `command_name`
- `command_path`
- `extensions`

## 生命周期 hooks

hooks 适合做执行前后控制，而不是包装式横切逻辑。

### App 级 hook

```go
app.BeforeRun = func(ctx cmd.HookContext) error {
	return nil
}

app.AfterRun = func(ctx cmd.HookContext) {
	if ctx.Err != nil {
		log.Printf("command failed: %v", ctx.Err)
	}
}

app.OnError = func(ctx cmd.HookContext) {
	var cliErr *cmd.CLIError
	if errors.As(ctx.Err, &cliErr) {
		log.Printf("kind=%s exit=%d command=%s", cliErr.Kind, cliErr.ExitCode, cliErr.Command)
	}
}
```

### Command 级 hook

```go
cmdDeploy := &cmd.Command{
	Name: "deploy",
	BeforeRun: func(ctx cmd.HookContext) error {
		return nil
	},
	AfterRun: func(ctx cmd.HookContext) {},
	OnError:  func(ctx cmd.HookContext) {},
	Run:      runDeploy,
}
```

### 调用顺序

成功时：

1. app `BeforeRun`
2. command `BeforeRun`
3. `Run`
4. command `AfterRun`
5. app `AfterRun`

失败时：

1. app `BeforeRun`
2. command `BeforeRun`
3. `Run` 或 hook 返回错误
4. command `OnError`
5. app `OnError`
6. command `AfterRun`
7. app `AfterRun`

## middleware

middleware 适合横切逻辑，例如：

- 鉴权
- tracing
- 限流
- 审计
- 统一日志

### App 级 middleware

```go
app.Use(func(ctx cmd.MiddlewareContext, next cmd.NextFunc) error {
	start := time.Now()
	err := next(ctx.Context)
	log.Printf("command=%s duration=%s err=%v", ctx.Command.Name, time.Since(start), err)
	return err
})
```

### Command 级 middleware

```go
cmdDeploy := &cmd.Command{
	Name: "deploy",
	Middlewares: []cmd.Middleware{
		func(ctx cmd.MiddlewareContext, next cmd.NextFunc) error {
			if len(ctx.Args) == 0 {
				return errors.New("missing target")
			}
			return next(ctx.Context)
		},
	},
	Run: runDeploy,
}
```

### 包裹顺序

执行顺序是：

`app middleware -> command middleware -> Command.Run`

## observer 与 telemetry

observer 提供稳定的事件流，适合接：

- metrics
- tracing adapter
- event log
- analytics

### 注册 observer

```go
app.AddObserver(cmd.ObserverFunc(func(event cmd.Event) {
	log.Printf(
		"type=%s command=%s exit=%d duration=%s",
		event.Type,
		event.Command.Name,
		event.ExitCode,
		event.Duration,
	)
}))
```

命令自身也可以挂 `Observers`。

### 当前事件类型

- `command_started`
- `command_finished`
- `command_failed`

### 事件字段

`Event` 包含：

- `Type`
- `App`
- `Command`
- `Args`
- `Err`
- `StartTime`
- `EndTime`
- `Duration`
- `ExitCode`

## 统一错误与退出码

库内部归一化错误会返回 `*CLIError`。

### 错误类型

当前 `Kind` 包括：

- `invalid_arguments`
- `not_found`
- `canceled`
- `internal`
- `runtime`

### 退出码

- 参数错误：`2`
- 未知命令：`2`
- `context.Canceled`：`130`
- `context.DeadlineExceeded`：`124`
- 运行期错误：`1`

### 示例

```go
err := app.Run(ctx, os.Args[1:])
if err != nil {
	var cliErr *cmd.CLIError
	if errors.As(err, &cliErr) {
		fmt.Println(cliErr.Kind, cliErr.ExitCode, cliErr.Command)
	}
}
```

`Execute()` 会在默认实例上自动按 `ExitStatus()` 退出。

## 自定义扩展 metadata

如果你需要给命令树挂自定义信息，例如：

- 站点分类
- 控制台 UI 提示
- 内部 owner
- feature flag
- OpenAPI / agent 扩展字段

可以使用 `extensions`。

### App 级

```go
app.SetExtension("x-site-section", "cli")
```

### Command 级

```go
cmdDeploy.SetExtension("x-owner", "platform")
```

### Positional 级

```go
cmdDeploy.Positionals[0].SetExtension("x-label", "Environment")
```

### Flag 级

```go
cmdDeploy.SetFlags = func(f *cmd.FlagSet) {
	f.StringVar(&format, "format", "", "output format", "")
	_ = f.SetExtension("format", "x-ui-control", "select")
}
```

### Surface 级 override

如果同一条命令在 CLI 和 REPL/runtime schema 下需要不同导出形态，建议保留一套基础命令定义，再挂 per-surface override：

```go
requiredFalse := false

cmdCreateUser := &cmd.Command{
	Name:      "user",
	UsageLine: "app create user <name> [flags]",
	Positionals: []cmd.PositionalArg{{
		Name:          "name",
		Usage:         "user name",
		Required:      true,
		Kind:          "user",
		CompletionKey: "user",
		Surfaces: map[cmd.Surface]cmd.PositionalSurface{
			cmd.SurfaceREPL: {Required: &requiredFalse},
		},
	}},
	Surfaces: map[cmd.Surface]cmd.CommandSurface{
		cmd.SurfaceREPL: {UsageLine: "app create user [name] [flags]"},
	},
}
```

这些字段会进入：

- `spec`
- Markdown frontmatter

Extensions 在复制到 spec、docs 和运行期 flag view 时会被 clone。map 和 slice 形态的值会递归 clone，但 opaque pointer 或自定义对象 payload 会按引用共享。如果你需要完全隔离，请放入 immutable value，或在写入 Extensions 前自行 clone payload。

## 常见模式

### 1. 全局配置 + 命令参数

```go
app := cmd.NewApp("app")
app.EnableConfigSupport()

app.SetFlags = func(f *cmd.FlagSet) {
	f.BoolVar(&verbose, "verbose", false, "verbose output", "v")
}

app.Commands = []*cmd.Command{
	{
		Name: "sync",
		SetFlags: func(f *cmd.FlagSet) {
			f.StringVar(&endpoint, "endpoint", "", "api endpoint", "")
			f.BindEnv("endpoint", "APP_ENDPOINT")
			f.BindConfig("endpoint", "api.endpoint")
			f.MarkRequired("endpoint")
		},
		Run: runSync,
	},
}
```

### 2. 用 enum 驱动 completion 和校验

```go
f.StringVar(&env, "env", "", "target environment", "")
f.SetEnum("env", "dev", "staging", "prod")
```

### 3. 用 observer 接 metrics

```go
app.AddObserver(cmd.ObserverFunc(func(event cmd.Event) {
	switch event.Type {
	case cmd.EventCommandFinished:
		metrics.RecordSuccess(event.Command.Name, event.Duration)
	case cmd.EventCommandFailed:
		metrics.RecordFailure(event.Command.Name, event.Duration)
	}
}))
```

### 4. 用 docs/spec 驱动站点和控制台

```bash
app spec json > site/spec.json
app docs markdown ./site/docs
```

## API 速查

### 应用与命令

- `NewApp(name string) *App`
- `(*App).Run(ctx, args)`
- `(*App).RunWith(ctx, runtime)`
- `(*App).RunAuto(ctx, args)`
- `(*App).RunDefault(ctx, args)`
- `(*App).RunLine(ctx, line)`
- `(*App).RunREPL(ctx, in, out)`
- `(*App).Main(ctx, runtime)`
- `(*App).MainAuto(ctx, args)`
- `(*App).MainDefault(ctx, args)`
- `(*App).MustRun(ctx, runtime)`
- `(*App).MustRunAuto(ctx, args)`
- `(*App).MustRunDefault(ctx, args)`
- `(*App).DefaultRuntime(args)`
- `(*App).CompleteLine(line, cursor)`
- `(*App).CompleteLineDetailed(line, cursor)`
- `(*App).EnableREPL()`
- `(*App).ConfigureREPL(fn)`
- `(*App).EnableConfigSupport()`
- `(*App).Use(...)`
- `(*App).AddObserver(...)`
- `(*App).SetExtension(key, value)`
- `(*App).Spec()`
- `(*App).SpecFor(surface)`
- `(*App).AvailableSurfaces()`

### 默认实例

- `SetFlags(...)`
- `AddCommands(...)`
- `SetUsageTemplate(...)`
- `Execute()`
- `Main(app)`
- `MainWithContext(ctx, app)`

### FlagSet 元数据

- `BindEnv`
- `BindConfig`
- `SetID`
- `SetKind`
- `SetEnum`
- `SetCompletionKey`
- `SetCompletion`
- `MarkRepeatable`
- `MarkRequired`
- `MarkHidden`
- `MarkDeprecated`
- `SetCategory`
- `SetExample`
- `SetExtension`
- `SetSurface`

### 共用错误与建议 helper

- `SuggestCommands`
- `UnknownCommandError`
- `UnknownSubcommandError`
- `UsageError`

### 相关类型

- `App`
- `Command`
- `REPL`
- `REPLConfig`
- `Runtime`
- `DefaultRuntime`
- `CLIRuntime`
- `REPLRuntime`
- `AutoRuntime`
- `Surface`
- `CommandSurface`
- `PositionalSurface`
- `FlagSurface`
- `FlagSet`
- `Flag`
- `PositionalArg`
- `CompletionContext`
- `CompletionResult`
- `LineCompleter`
- `DetailedLineCompleter`
- `REPL`
- `HookContext`
- `MiddlewareContext`
- `Event`
- `CLIError`
- `AppSpec`

## 总结

如果你的目标只是做一个简单 CLI，使用：

- `SetFlags`
- `AddCommands`
- `Execute`

就足够了。

如果你的目标是做一个可以长期演进、能接 completion、文档、站点、控制台和 agent 的 CLI 平台，那么建议直接围绕下面这条链路组织：

- `Command / Flag / Positional` 作为统一命令模型
- `env / config / CLI` 作为统一配置来源
- `hooks / middleware / observer` 作为统一运行时扩展点
- `spec / docs` 作为统一外部契约
- `surface override + rich spec metadata` 作为连接 REPL、parser、schema、agent consumer 的桥梁

这也是这个库当前最适合的使用方式。
