package cmd

import (
	"errors"
	"strings"
	"unicode"
)

var (
	errTrailingEscape = errors.New("trailing escape")
	errUnclosedQuote  = errors.New("unclosed quote")
	errUnclosedSingle = errors.New("unclosed single quote")
	errUnclosedDouble = errors.New("unclosed double quote")
)

type splitState struct {
	tokens           []string
	current          strings.Builder
	tokenStarted     bool
	trailingBoundary bool
	inSingle         bool
	inDouble         bool
	escaped          bool
}

// SplitLine splits a REPL input line into argv-style tokens.
func SplitLine(line string) ([]string, error) {
	state := parseSplitState(line)

	if state.escaped {
		return nil, errTrailingEscape
	}
	if state.inSingle {
		return nil, errUnclosedSingle
	}
	if state.inDouble {
		return nil, errUnclosedDouble
	}
	if state.tokenStarted {
		state.tokens = append(state.tokens, state.current.String())
	}
	return state.tokens, nil
}

// SplitLineForCompletion splits a partial REPL line into completed args and the current token.
func SplitLineForCompletion(line string) (args []string, current string, err error) {
	state := parseSplitState(line)

	if state.tokenStarted {
		state.tokens = append(state.tokens, state.current.String())
	}

	if state.trailingBoundary {
		return state.tokens, "", nil
	}
	if len(state.tokens) == 0 {
		return nil, "", nil
	}
	return state.tokens[:len(state.tokens)-1], state.tokens[len(state.tokens)-1], nil
}

func parseSplitState(line string) splitState {
	state := splitState{}

	for _, r := range line {
		if state.escaped {
			state.current.WriteRune(r)
			state.tokenStarted = true
			state.trailingBoundary = false
			state.escaped = false
			continue
		}

		if r == '\\' {
			state.escaped = true
			state.tokenStarted = true
			state.trailingBoundary = false
			continue
		}

		if state.inSingle {
			if r == '\'' {
				state.inSingle = false
			} else {
				state.current.WriteRune(r)
			}
			state.tokenStarted = true
			state.trailingBoundary = false
			continue
		}

		if state.inDouble {
			if r == '"' {
				state.inDouble = false
			} else {
				state.current.WriteRune(r)
			}
			state.tokenStarted = true
			state.trailingBoundary = false
			continue
		}

		switch {
		case unicode.IsSpace(r):
			if state.tokenStarted {
				state.tokens = append(state.tokens, state.current.String())
				state.current.Reset()
				state.tokenStarted = false
			}
			state.trailingBoundary = true
		case r == '\'':
			state.inSingle = true
			state.tokenStarted = true
			state.trailingBoundary = false
		case r == '"':
			state.inDouble = true
			state.tokenStarted = true
			state.trailingBoundary = false
		default:
			state.current.WriteRune(r)
			state.tokenStarted = true
			state.trailingBoundary = false
		}
	}

	return state
}
