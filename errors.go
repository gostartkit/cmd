package cmd

import (
	"context"
	"errors"
	"fmt"
)

const (
	ErrorKindInvalidArguments = "invalid_arguments"
	ErrorKindNotFound         = "not_found"
	ErrorKindCanceled         = "canceled"
	ErrorKindInternal         = "internal"
	ErrorKindRuntime          = "runtime"
)

type CLIError struct {
	Kind     string
	Message  string
	Command  string
	ExitCode int
	Err      error
}

func (e *CLIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "command failed"
}

func (e *CLIError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func normalizeError(err error, command string, fallbackKind string, fallbackExitCode int) *CLIError {
	if err == nil {
		return nil
	}

	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		if cliErr.Command == "" {
			cliErr.Command = command
		}
		if cliErr.ExitCode == 0 {
			cliErr.ExitCode = fallbackExitCode
		}
		if cliErr.Kind == "" {
			cliErr.Kind = fallbackKind
		}
		return cliErr
	}

	switch {
	case errors.Is(err, context.Canceled):
		return &CLIError{
			Kind:     ErrorKindCanceled,
			Message:  err.Error(),
			Command:  command,
			ExitCode: 130,
			Err:      err,
		}
	case errors.Is(err, context.DeadlineExceeded):
		return &CLIError{
			Kind:     ErrorKindCanceled,
			Message:  err.Error(),
			Command:  command,
			ExitCode: 124,
			Err:      err,
		}
	case errors.Is(err, ErrNotFound):
		return &CLIError{
			Kind:     ErrorKindNotFound,
			Message:  err.Error(),
			Command:  command,
			ExitCode: 2,
			Err:      err,
		}
	default:
		message := err.Error()
		if message == "" {
			message = fmt.Sprintf("%s error", fallbackKind)
		}
		return &CLIError{
			Kind:     fallbackKind,
			Message:  message,
			Command:  command,
			ExitCode: fallbackExitCode,
			Err:      err,
		}
	}
}
