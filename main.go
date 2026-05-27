package main

import (
	"context"
	"embed"
	"log"

	"solderdb/internal/bridge"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist/*
var assets embed.FS

type App struct {
	ctx context.Context
	svc *bridge.DBService
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(ctx context.Context) {
	if a.svc != nil {
		_ = a.svc.Close()
	}
}

func main() {
	app := NewApp()

	svc, err := bridge.NewDBService("")
	if err != nil {
		log.Fatal(err)
	}
	app.svc = svc

	if err := wails.Run(&options.App{
		Title:  "SolderDB",
		Width:  1100,
		Height: 750,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
			// Expose DB service directly for the frontend bindings.
			app.svc,
		},
	}); err != nil {
		log.Fatal(err)
	}
}
