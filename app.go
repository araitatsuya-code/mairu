package main

import (
	"context"

	"mairu/internal/types"
)

type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) AppName() string {
	return "Mairu"
}

// GetRuntimeStatus は起動時に必要な初期状態を返す。
func (a *App) GetRuntimeStatus() types.RuntimeStatus {
	return types.RuntimeStatus{
		Authorized:       false,
		ClaudeConfigured: false,
		DatabaseReady:    false,
		LastRunAt:        nil,
	}
}
