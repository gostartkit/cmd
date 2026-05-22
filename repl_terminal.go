package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"pkg.gostartkit.com/cmd/internal/terminal"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

type TerminalREPLDriver struct{}

const (
	ansiDim    = "\033[90m"
	ansiBlue   = "\033[94m"
	ansiGreen  = "\033[92m"
	ansiYellow = "\033[93m"
	ansiCyan   = "\033[96m"
	ansiReset  = "\033[0m"

	completionPageSize = 8
)

type replTerminalSession struct {
	repl       *REPL
	in         *os.File
	out        *os.File
	line       []rune
	cursor     int
	history    []string
	historyPos int
	scratch    []rune

	completionCycle completionCycleState
}

type completionCycleState struct {
	line   string
	cursor int
	page   int
}

func isTTYREPL(r *REPL) bool {
	if r == nil {
		return false
	}
	in, okIn := r.In.(*os.File)
	out, okOut := r.Out.(*os.File)
	if !okIn || !okOut {
		return false
	}
	return terminal.IsTerminalFD(int(in.Fd())) && terminal.IsTerminalFD(int(out.Fd()))
}

func (d TerminalREPLDriver) Run(ctx context.Context, repl *REPL) error {
	in, okIn := repl.In.(*os.File)
	out, okOut := repl.Out.(*os.File)
	if !okIn || !okOut {
		return BasicREPLDriver{}.Run(ctx, repl)
	}

	oldState, err := terminal.MakeRawFD(int(in.Fd()))
	if err != nil {
		return BasicREPLDriver{}.Run(ctx, repl)
	}
	defer terminal.RestoreFD(int(in.Fd()), oldState)

	session := &replTerminalSession{
		repl:       repl,
		in:         in,
		out:        out,
		historyPos: 0,
	}
	session.resetLine()
	return session.loop(ctx)
}

func (s *replTerminalSession) loop(ctx context.Context) error {
	if err := s.render(); err != nil {
		return err
	}

	buf := make([]byte, 1)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		n, err := s.in.Read(buf)
		if err != nil {
			if err == io.EOF {
				fmt.Fprint(s.out, "\r\n")
				return nil
			}
			return err
		}
		if n == 0 {
			continue
		}

		switch buf[0] {
		case '\r', '\n':
			if err := s.submit(ctx); err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				return err
			}
		case 3:
			if err := s.interrupt(); err != nil {
				return err
			}
		case 4:
			if len(s.line) == 0 {
				fmt.Fprint(s.out, "\r\n")
				return nil
			}
			if err := s.deleteAtCursor(); err != nil {
				return err
			}
		case 9:
			if err := s.complete(); err != nil {
				return err
			}
		case 127, 8:
			if err := s.backspace(); err != nil {
				return err
			}
		case 1:
			s.cursor = 0
			if err := s.render(); err != nil {
				return err
			}
		case 5:
			s.cursor = len(s.line)
			if err := s.render(); err != nil {
				return err
			}
		case 27:
			if err := s.handleEscape(); err != nil {
				return err
			}
		default:
			if buf[0] < utf8.RuneSelf {
				if buf[0] < 32 || !unicode.IsPrint(rune(buf[0])) {
					continue
				}
				if err := s.insertRune(rune(buf[0])); err != nil {
					return err
				}
				continue
			}
			r, err := s.readRune(buf[0])
			if err != nil {
				return err
			}
			if !unicode.IsPrint(r) {
				continue
			}
			if err := s.insertRune(r); err != nil {
				return err
			}
		}
	}
}

func (s *replTerminalSession) readRune(first byte) (rune, error) {
	var seq [utf8.UTFMax]byte
	seq[0] = first
	for n := 1; n <= utf8.UTFMax; n++ {
		if utf8.FullRune(seq[:n]) {
			r, size := utf8.DecodeRune(seq[:n])
			if r != utf8.RuneError || size > 1 {
				return r, nil
			}
		}
		if n == utf8.UTFMax {
			break
		}
		if _, err := s.in.Read(seq[n : n+1]); err != nil {
			return utf8.RuneError, err
		}
	}
	return utf8.RuneError, nil
}

func (s *replTerminalSession) submit(ctx context.Context) error {
	line := string(s.line)
	fmt.Fprint(s.out, "\r\n")
	if strings.TrimSpace(line) != "" {
		s.appendHistory(line)
	}
	exit, err := s.repl.handleLine(ctx, line)
	if exit {
		return io.EOF
	}
	if err != nil {
		fmt.Fprintf(s.repl.Err, "Error: %v\n", err)
	}
	s.resetLine()
	return s.render()
}

func (s *replTerminalSession) interrupt() error {
	fmt.Fprint(s.out, "^C\r\n")
	s.resetLine()
	return s.render()
}

func (s *replTerminalSession) backspace() error {
	if s.cursor == 0 {
		return nil
	}
	s.resetCompletionCycle()
	s.line = slices.Delete(s.line, s.cursor-1, s.cursor)
	s.cursor--
	return s.render()
}

func (s *replTerminalSession) deleteAtCursor() error {
	if s.cursor >= len(s.line) {
		return nil
	}
	s.resetCompletionCycle()
	s.line = slices.Delete(s.line, s.cursor, s.cursor+1)
	return s.render()
}

func (s *replTerminalSession) insertRune(r rune) error {
	s.resetCompletionCycle()
	s.line = slices.Insert(s.line, s.cursor, r)
	s.cursor++
	return s.render()
}

func (s *replTerminalSession) handleEscape() error {
	seq := make([]byte, 2)
	n, err := s.in.Read(seq[:1])
	if err != nil || n == 0 {
		return err
	}
	if seq[0] != '[' {
		return nil
	}
	n, err = s.in.Read(seq[1:2])
	if err != nil || n == 0 {
		return err
	}

	switch seq[1] {
	case 'A':
		return s.historyUp()
	case 'B':
		return s.historyDown()
	case 'C':
		if s.cursor < len(s.line) {
			s.cursor++
			return s.render()
		}
	case 'D':
		if s.cursor > 0 {
			s.cursor--
			return s.render()
		}
	case '3':
		tilde := make([]byte, 1)
		if _, err := s.in.Read(tilde); err != nil {
			return err
		}
		if tilde[0] == '~' {
			return s.deleteAtCursor()
		}
	}
	return nil
}

func (s *replTerminalSession) historyUp() error {
	if len(s.history) == 0 {
		return nil
	}
	s.resetCompletionCycle()
	if s.historyPos == len(s.history) {
		s.scratch = append([]rune(nil), s.line...)
	}
	if s.historyPos > 0 {
		s.historyPos--
	}
	s.line = []rune(s.history[s.historyPos])
	s.cursor = len(s.line)
	return s.render()
}

func (s *replTerminalSession) historyDown() error {
	if len(s.history) == 0 {
		return nil
	}
	s.resetCompletionCycle()
	if s.historyPos < len(s.history)-1 {
		s.historyPos++
		s.line = []rune(s.history[s.historyPos])
	} else {
		s.historyPos = len(s.history)
		s.line = append([]rune(nil), s.scratch...)
	}
	s.cursor = len(s.line)
	return s.render()
}

func (s *replTerminalSession) complete() error {
	line := string(s.line)
	cursorBytes := utf8CursorOffset(s.line, s.cursor)
	results := s.repl.App.CompleteLineDetailed(line, cursorBytes)
	if len(results) == 0 {
		_, err := fmt.Fprint(s.out, "\a")
		return err
	}

	start, current := completionReplaceStart(s.line, s.cursor)
	if len(results) == 1 {
		s.resetCompletionCycle()
		s.applyCompletion(start, current, results[0].Value, true)
		return s.render()
	}

	common := longestCommonCompletionPrefix(results)
	if common != "" && common != current {
		s.resetCompletionCycle()
		s.applyCompletion(start, current, common, false)
		return s.render()
	}

	page := s.nextCompletionPage(line, s.cursor, len(results))
	fmt.Fprint(s.out, "\r\n")
	for _, result := range completionPage(results, page, completionPageSize) {
		fmt.Fprintln(s.out, formatCompletionDisplayLine(result))
	}
	if footer := formatCompletionPageFooter(results, page, completionPageSize); footer != "" {
		fmt.Fprintln(s.out, footer)
	}
	return s.render()
}

func (s *replTerminalSession) applyCompletion(start int, current string, value string, addTrailingSpace bool) {
	prefixLen := len([]rune(current))
	end := start + prefixLen
	s.line = append(append(append([]rune(nil), s.line[:start]...), []rune(value)...), s.line[end:]...)
	s.cursor = start + len([]rune(value))
	if addTrailingSpace && s.cursor == len(s.line) {
		s.line = append(s.line, ' ')
		s.cursor++
	}
}

func (s *replTerminalSession) appendHistory(line string) {
	if len(s.history) == 0 || s.history[len(s.history)-1] != line {
		s.history = append(s.history, line)
	}
	s.historyPos = len(s.history)
	s.scratch = nil
}

func (s *replTerminalSession) resetLine() {
	s.line = s.line[:0]
	s.cursor = 0
	s.historyPos = len(s.history)
	s.scratch = nil
	s.resetCompletionCycle()
}

func (s *replTerminalSession) resetCompletionCycle() {
	s.completionCycle = completionCycleState{}
}

func (s *replTerminalSession) nextCompletionPage(line string, cursor int, resultCount int) int {
	if resultCount <= 0 {
		s.resetCompletionCycle()
		return 0
	}
	totalPages := completionPageCount(resultCount, completionPageSize)
	if totalPages <= 1 {
		s.completionCycle = completionCycleState{line: line, cursor: cursor, page: 0}
		return 0
	}

	if s.completionCycle.line == line && s.completionCycle.cursor == cursor {
		s.completionCycle.page = (s.completionCycle.page + 1) % totalPages
		return s.completionCycle.page
	}

	s.completionCycle = completionCycleState{
		line:   line,
		cursor: cursor,
		page:   0,
	}
	return 0
}

func (s *replTerminalSession) render() error {
	line := string(s.line)
	hint := s.currentHint()
	ghost := s.currentGhostText()
	if _, err := fmt.Fprintf(s.out, "\r\033[2K%s%s%s\n\033[2K%s\033[1A\r", s.repl.Prompt, line, ghost, hint); err != nil {
		return err
	}
	cursorCol := len([]rune(s.repl.Prompt)) + s.cursor
	if cursorCol > 0 {
		if _, err := fmt.Fprintf(s.out, "\033[%dC", cursorCol); err != nil {
			return err
		}
	}
	return nil
}

func (s *replTerminalSession) currentHint() string {
	if s == nil || s.repl == nil || s.repl.App == nil {
		return ""
	}
	results := s.repl.App.CompleteLineDetailed(string(s.line), utf8CursorOffset(s.line, s.cursor))
	return formatCompletionHint(results)
}

func (s *replTerminalSession) currentGhostText() string {
	if s == nil || s.repl == nil || s.repl.App == nil || s.cursor != len(s.line) {
		return ""
	}
	results := s.repl.App.CompleteLineDetailed(string(s.line), utf8CursorOffset(s.line, s.cursor))
	return formatCompletionGhostText(s.line, s.cursor, results)
}

func utf8CursorOffset(line []rune, cursor int) int {
	if cursor <= 0 {
		return 0
	}
	if cursor > len(line) {
		cursor = len(line)
	}
	return len(string(line[:cursor]))
}

func completionReplaceStart(line []rune, cursor int) (int, string) {
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(line) {
		cursor = len(line)
	}

	start := cursor
	tokenStarted := false
	trailingBoundary := false
	inSingle := false
	inDouble := false
	escaped := false

	for i, r := range line[:cursor] {
		if escaped {
			if !tokenStarted {
				start = i
			}
			tokenStarted = true
			trailingBoundary = false
			escaped = false
			continue
		}

		if r == '\\' {
			if !tokenStarted {
				start = i
			}
			tokenStarted = true
			trailingBoundary = false
			escaped = true
			continue
		}

		if inSingle {
			if r == '\'' {
				inSingle = false
			}
			tokenStarted = true
			trailingBoundary = false
			continue
		}

		if inDouble {
			if r == '"' {
				inDouble = false
			}
			tokenStarted = true
			trailingBoundary = false
			continue
		}

		switch {
		case unicode.IsSpace(r):
			tokenStarted = false
			trailingBoundary = true
			start = i + 1
		case r == '\'':
			if !tokenStarted {
				start = i + 1
			}
			tokenStarted = true
			inSingle = true
			trailingBoundary = false
		case r == '"':
			if !tokenStarted {
				start = i + 1
			}
			tokenStarted = true
			inDouble = true
			trailingBoundary = false
		default:
			if !tokenStarted {
				start = i
			}
			tokenStarted = true
			trailingBoundary = false
		}
	}

	if trailingBoundary || !tokenStarted {
		return cursor, ""
	}
	return start, string(line[start:cursor])
}

func longestCommonCompletionPrefix(results []CompletionResult) string {
	if len(results) == 0 {
		return ""
	}
	common := results[0].Value
	for _, result := range results[1:] {
		common = sharedPrefix(common, result.Value)
		if common == "" {
			return ""
		}
	}
	return common
}

func sharedPrefix(a string, b string) string {
	ar := []rune(a)
	br := []rune(b)
	limit := len(ar)
	if len(br) < limit {
		limit = len(br)
	}
	i := 0
	for i < limit && ar[i] == br[i] {
		i++
	}
	return string(ar[:i])
}

func formatCompletionHint(results []CompletionResult) string {
	if len(results) == 0 {
		return ""
	}
	if len(results) == 1 {
		if results[0].Description != "" {
			return "hint: " + results[0].Value + " - " + results[0].Description
		}
		return "hint: " + results[0].Value
	}

	limit := 3
	if len(results) < limit {
		limit = len(results)
	}
	values := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		values = append(values, results[i].Value)
	}
	hint := "hint: " + strings.Join(values, ", ")
	if len(results) > limit {
		hint += fmt.Sprintf(" (+%d more)", len(results)-limit)
	}
	return hint
}

func formatCompletionGhostText(line []rune, cursor int, results []CompletionResult) string {
	if len(results) == 0 || cursor != len(line) {
		return ""
	}

	_, current := completionReplaceStart(line, cursor)
	if current == "" {
		return ""
	}

	suggestion := ""
	if len(results) == 1 {
		suggestion = results[0].Value
	} else {
		suggestion = longestCommonCompletionPrefix(results)
	}
	if suggestion == "" || suggestion == current {
		return ""
	}

	currentRunes := []rune(current)
	suggestionRunes := []rune(suggestion)
	if len(suggestionRunes) <= len(currentRunes) {
		return ""
	}
	for i := range currentRunes {
		if suggestionRunes[i] != currentRunes[i] {
			return ""
		}
	}

	return ansiDim + string(suggestionRunes[len(currentRunes):]) + ansiReset
}

func formatCompletionDisplayLine(result CompletionResult) string {
	tag := completionKindTag(result.Kind)
	color := completionKindColor(result.Kind)
	if result.Description == "" {
		return fmt.Sprintf("%s[%s]%s %s", color, tag, ansiReset, result.Value)
	}
	return fmt.Sprintf("%s[%s]%s %-20s %s", color, tag, ansiReset, result.Value, result.Description)
}

func completionKindTag(kind string) string {
	switch kind {
	case completionKindCommand:
		return "cmd"
	case completionKindFlag:
		return "flag"
	case completionKindValue:
		return "value"
	case completionKindPositional:
		return "arg"
	case completionKindBuiltin:
		return "builtin"
	default:
		return "item"
	}
}

func completionKindColor(kind string) string {
	switch kind {
	case completionKindCommand:
		return ansiBlue
	case completionKindFlag:
		return ansiGreen
	case completionKindValue:
		return ansiYellow
	case completionKindPositional:
		return ansiCyan
	case completionKindBuiltin:
		return ansiDim
	default:
		return ansiReset
	}
}

func completionPage(results []CompletionResult, page int, pageSize int) []CompletionResult {
	if len(results) == 0 {
		return nil
	}
	if pageSize <= 0 || len(results) <= pageSize {
		return append([]CompletionResult(nil), results...)
	}
	totalPages := completionPageCount(len(results), pageSize)
	if totalPages <= 0 {
		return append([]CompletionResult(nil), results...)
	}
	if page < 0 {
		page = 0
	}
	page = page % totalPages
	start := page * pageSize
	end := start + pageSize
	if end > len(results) {
		end = len(results)
	}
	return append([]CompletionResult(nil), results[start:end]...)
}

func completionPageCount(total int, pageSize int) int {
	if total <= 0 {
		return 0
	}
	if pageSize <= 0 {
		return 1
	}
	return (total + pageSize - 1) / pageSize
}

func formatCompletionPageFooter(results []CompletionResult, page int, pageSize int) string {
	if len(results) == 0 {
		return ""
	}
	totalPages := completionPageCount(len(results), pageSize)
	if totalPages <= 1 {
		return ""
	}
	if page < 0 {
		page = 0
	}
	page = page % totalPages
	start := page*pageSize + 1
	end := start + pageSize - 1
	if end > len(results) {
		end = len(results)
	}
	return fmt.Sprintf("%shint: showing %d-%d of %d (page %d/%d, Tab for more)%s", ansiDim, start, end, len(results), page+1, totalPages, ansiReset)
}
