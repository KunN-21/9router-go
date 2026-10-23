package main

import (
	_ "embed"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/getlantern/systray"

	"9router/proxy/internal/tray"
)

//go:embed tray.ico
var trayIcon []byte

// The menu is rebuilt on every open rather than mutated in place: a menu that
// caches the state it drew is how a tray ends up saying "Running" about a
// gateway that stopped ten minutes ago. Rebuilding costs microseconds and
// cannot go stale.
func runGUI(actions tray.Actions, port int) error {
	systray.Run(func() {
		systray.SetIcon(trayIcon)
		systray.SetTitle("9Router")
		go menuLoop(actions, port)
	}, func() {
		// Quit Tray: release the icon and return. Deliberately empty — C4 says
		// quitting the tray must not touch the gateway, and the daemon has
		// already been released by whoever started it, so there is no handle
		// here to close.
	})
	return nil
}

func menuLoop(actions tray.Actions, port int) {
	snap := actions.Snapshot()
	systray.SetTooltip(snap.Title())

	systray.AddMenuItem(snap.Title(), "").Disable()

	if snap.CanStart() {
		if item := systray.AddMenuItem("Start Gateway", ""); item != nil {
			go func() {
				<-item.ClickedCh
				report(actions.Start())
			}()
		}
	}

	if snap.CanStop() {
		if item := systray.AddMenuItem("Stop Gateway", ""); item != nil {
			go func() {
				<-item.ClickedCh
				report(actions.Stop())
			}()
		}
	}

	open := systray.AddMenuItem("Open Dashboard", "")
	go func() {
		<-open.ClickedCh
		openDashboard(port)
	}()

	label := "Auto-start on Boot"
	if actions.AutostartEnabled() {
		label = "✓ Auto-start on Boot"
	}
	auto := systray.AddMenuItem(label, "")
	go func() {
		<-auto.ClickedCh
		// Read again rather than trusting the label: the entry may have been
		// changed by the CLI between the menu drawing and the click.
		report(actions.SetAutostart(!actions.AutostartEnabled()))
	}()

	quit := systray.AddMenuItem("Quit Tray", "")
	go func() {
		<-quit.ClickedCh
		systray.Quit()
	}()
}

// report surfaces a failure without ever printing a credential (C6): errors
// from the actions carry a command and its output, never a token.
func report(err error) {
	if err == nil {
		return
	}
	fmt.Println(err)
}

func openDashboard(port int) {
	url := fmt.Sprintf("http://127.0.0.1:%d/dashboard", port)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
