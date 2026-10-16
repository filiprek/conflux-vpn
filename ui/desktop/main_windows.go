//go:build windows

package main

import (
	"embed"
	"fmt"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"os"
	"runtime"
)

//go:embed all:frontend/dist
var assets embed.FS
var version = "dev"
var commit = "unknown"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Printf("Conflux %s (%s) %s protocol 1\n", version, commit, runtime.Version())
		return
	}
	err := wails.Run(&options.App{Title: "Conflux", Width: 1180, Height: 820, MinWidth: 800, MinHeight: 600, AssetServer: &assetserver.Options{Assets: assets}, Bind: []interface{}{&App{}}, BackgroundColour: &options.RGBA{R: 244, G: 246, B: 248, A: 255}})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Conflux desktop could not start")
		os.Exit(1)
	}
}
