# Development setup (Windows)

This repo is intended to run as a Wails v2 desktop app:
- Backend: Go
- Frontend: React + TypeScript + Tailwind

## Install prerequisites

1) Install Go (1.22+).
2) Install Wails CLI v2:

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Ensure `%GOPATH%\bin` is on PATH so `wails` is discoverable.

## Run dev

From the repo root:

```powershell
wails dev
```

## Generate frontend bindings

Wails generates TS bindings during `wails dev` / `wails build`. After the first run,
import the generated bindings from the frontend and keep TypeScript strict (no `any`).

