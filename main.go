package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if err := loadLocalEnv(defaultEnvFile); err != nil {
		println("Warning:", err.Error())
	}

	app := NewApp()

	err := wails.Run(&options.App{
		Title:            "Mairu",
		Width:            1200,
		Height:           820,
		MinWidth:         960,
		MinHeight:        640,
		DisableResize:    false,
		Frameless:        false,
		BackgroundColour: &options.RGBA{R: 15, G: 23, B: 42, A: 1},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
