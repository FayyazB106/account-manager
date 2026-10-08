//go:build darwin

package main

import (
	_ "embed"
	"log"
	"os/exec"

	"fyne.io/systray"
)

//go:embed mactray.png
var trayIcon []byte

// trayURL is the browser URL to open; used by the Dock/Finder "reopen" handler
// (reopen_darwin.go) when the app icon is clicked while it's already running.
var trayURL string

// reopenBrowser opens the app in the browser. Called from Cocoa when the running
// app's icon is clicked (macOS sends a reopen event rather than a new process).
func reopenBrowser() {
	if trayURL != "" {
		openBrowser(trayURL)
	}
}

// runTray shows a macOS menu-bar icon while the server runs in a background
// goroutine. Its presence is the "running" indicator: the icon exists only while
// this process (and thus the server) is alive. The menu opens the app in the
// browser at the ACTUAL listening URL and offers a clean Quit. runTray blocks on
// the main thread until Quit. (macOS uses Cocoa here, so the darwin build needs
// CGO — see the release workflow's package-macos job.)
func runTray(url string) {
	trayURL = url // for the reopen handler
	onReady := func() {
		// Template icon so it adapts to light/dark menu bars.
		systray.SetTemplateIcon(trayIcon, trayIcon)
		systray.SetTooltip("Account Manager — running\n" + url)

		mOpen := systray.AddMenuItem("Open Account Manager", "Open the app in your browser")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "Stop the server and exit")

		openBrowser(url) // open once on first launch

		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					openBrowser(url)
				case <-mQuit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}
	systray.Run(onReady, func() {})
}

func openBrowser(url string) {
	// Run (not Start) and wait: in the single-instance path the process exits
	// right after this, so the launch must complete first.
	if err := exec.Command("open", url).Run(); err != nil {
		log.Printf("open browser: %v", err)
	}
}
