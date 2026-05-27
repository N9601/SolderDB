package main

import (
	"context"
	"embed"
	"log"
	"time"

	"solderdb/internal/api"
	"solderdb/internal/auth"
	"solderdb/internal/bridge"
	"solderdb/internal/collections"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist/*
var assets embed.FS

type App struct {
	ctx context.Context
	svc *bridge.DBService
	api *api.Server
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(ctx context.Context) {
	if a.api != nil {
		stopCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		_ = a.api.Stop(stopCtx)
		cancel()
	}
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
	colls := bridge.NewCollectionsService(svc.Engine())
	var authBridge *bridge.AuthService

	// Start the local REST API on 127.0.0.1:8787 so SDKs/CLIs/curl can hit it.
	// AllowOrigin "*" is fine because we only listen on loopback.
	apiColls := collections.New(svc.Engine())
	authSvc, err := auth.New(svc.Engine(), apiColls, svc.Engine().DataDir())
	if err != nil {
		log.Fatalf("init auth: %v", err)
	}
	authBridge = bridge.NewAuthService(authSvc)
	apiSrv := api.New(svc.Engine(), apiColls, authSvc, api.Config{
		Addr:        "127.0.0.1:8787",
		AllowOrigin: "*",
	})
	if err := apiSrv.Start(); err != nil {
		log.Printf("api start failed: %v", err)
	} else {
		log.Printf("REST API listening on http://%s", apiSrv.Addr())
		app.api = apiSrv
		svc.SetAPIAddr("http://" + apiSrv.Addr())
	}

	if err := wails.Run(&options.App{
		Title:  "SolderDB",
		Width:  1280,
		Height: 820,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
			app.svc,
			colls,
			authBridge,
		},
	}); err != nil {
		log.Fatal(err)
	}
}
