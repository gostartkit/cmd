package cmd

import (
	"context"
	"time"
)

type MiddlewareContext struct {
	Context   context.Context
	App       *App
	Command   *Command
	Args      []string
	StartTime time.Time
}

type NextFunc func(ctx context.Context) error
type Middleware func(ctx MiddlewareContext, next NextFunc) error

func (a *App) Use(middlewares ...Middleware) {
	a.Middlewares = append(a.Middlewares, middlewares...)
}

func (c *Command) Use(middlewares ...Middleware) {
	c.Middlewares = append(c.Middlewares, middlewares...)
}

func chainMiddlewares(ctx MiddlewareContext, final NextFunc, middlewares []Middleware) error {
	if len(middlewares) == 0 {
		return final(ctx.Context)
	}

	next := final
	for i := len(middlewares) - 1; i >= 0; i-- {
		mw := middlewares[i]
		currentNext := next
		next = func(runCtx context.Context) error {
			mwCtx := ctx
			mwCtx.Context = runCtx
			return mw(mwCtx, currentNext)
		}
	}

	return next(ctx.Context)
}
