package cmd

import "time"

type EventType string

const (
	EventCommandStarted  EventType = "command_started"
	EventCommandFinished EventType = "command_finished"
	EventCommandFailed   EventType = "command_failed"
)

type Event struct {
	Type      EventType
	App       *App
	Command   *Command
	Args      []string
	Err       error
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
	ExitCode  int
}

type Observer interface {
	HandleEvent(event Event)
}

type ObserverFunc func(event Event)

func (f ObserverFunc) HandleEvent(event Event) {
	f(event)
}

func (a *App) AddObserver(observers ...Observer) {
	a.Observers = append(a.Observers, observers...)
}

func (c *Command) AddObserver(observers ...Observer) {
	c.Observers = append(c.Observers, observers...)
}

func (a *App) emitEvent(event Event) {
	if a == nil {
		return
	}
	event.App = a
	event.Args = append([]string(nil), event.Args...)

	for _, observer := range a.Observers {
		if observer != nil {
			observer.HandleEvent(event)
		}
	}
	if event.Command != nil {
		for _, observer := range event.Command.Observers {
			if observer != nil {
				observer.HandleEvent(event)
			}
		}
	}
}
