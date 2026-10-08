//go:build windows

package main

import (
	_ "embed"
	"log"
	"os/exec"

	"fyne.io/systray"
)

//go:embed appicon.ico
var trayIcon []byte

// runTray shows a Windows system-tray icon while the server runs in a background
// goroutine. Its presence is the "running" indicator: the icon exists only while
// this process (and thus the server) is alive. The menu opens the app in the
// browser at the ACTUAL listening URL (so a changed port always opens correctly)
// and offers a clean Quit. runTray blocks on the main thread until Quit.
func runTray(url string) {
	onReady := func() {
		systray.SetIcon(trayIcon)
		systray.SetTitle("Account Manager")
		systray.SetTooltip("Account Manager — running\n" + url)

		mOpen := systray.AddMenuItem("Open Account Manager", "Open the app in your browser")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "Stop the server and exit")

		// Open the browser once on startup so first launch shows the app.
		openBrowser(url)

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
	// onExit ends the process (which stops the server goroutine too).
	systray.Run(onReady, func() {})
}

func openBrowser(url string) {
	// "cmd /c start" hands the URL to the default browser. Use Run (not Start) and
	// wait for it to finish: in the single-instance path the process exits right
	// after this call, so the launch must complete first — otherwise the child is
	// killed before the browser opens (the "does nothing while already running" bug).
	if err := exec.Command("cmd", "/c", "start", "", url).Run(); err != nil {
		log.Printf("open browser: %v", err)
	}
}
