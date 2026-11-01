package main

import "pixelplay-launcher/host"

// App wraps host.App so Wails keeps binding as window.go.main.App.
type App struct {
	*host.App
}

func NewApp() *App {
	return &App{App: host.New()}
}
