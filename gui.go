//go:build darwin

package main

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"

	webview "github.com/webview/webview_go"
)

// uiAssets is the built frontend (ui/): rebuild it with `make ui` after changing it.
//
//go:embed all:ui/dist
var uiAssets embed.FS

// runUI opens the graphical UI in a native window: the embedded frontend is
// served from a local port and Go functions are bound into the page.
func runUI(args []string) {
	dir, _, err := startupPaths(args, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "v:", err)
		os.Exit(2)
	}
	applyDiffSettings(loadedSettings()) // the diff context of the settings, until they are changed here
	if err := openWindow(dir); err != nil {
		fmt.Fprintln(os.Stderr, "v:", err)
		os.Exit(1)
	}
}

func openWindow(startDir string) error {
	wd := &uiWorkdir{dir: startDir}
	root, err := fs.Sub(uiAssets, "ui/dist")
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()
	go http.Serve(ln, http.FileServerFS(root))

	w := webview.New(false)
	defer w.Destroy()
	// The agent's events go to the page as window.vEvent({...}).
	chat := newUIChat(func(e uiEvent) {
		b, _ := json.Marshal(e)
		w.Dispatch(func() { w.Eval("window.vEvent&&window.vEvent(" + string(b) + ")") })
	})
	defer chat.Reset()
	emit := func(e uiEvent) {
		b, _ := json.Marshal(e)
		w.Dispatch(func() { w.Eval("window.vEvent&&window.vEvent(" + string(b) + ")") })
	}
	shell := newUIShell(emit)
	defer shell.StopAll()
	w.SetTitle("Vibing")
	w.SetSize(1100, 720, webview.HintNone)
	// Bound functions appear as window.vStart() and window.vTagline().
	if err := w.Bind("vStart", func() uiStart { return newUIStart(wd.get()) }); err != nil {
		return err
	}
	if err := w.Bind("vTagline", randomTagline); err != nil {
		return err
	}
	if err := w.Bind("vOptions", func() []uiOption {
		s, _ := loadSettings()
		return uiOptions(s)
	}); err != nil {
		return err
	}
	// vSetOption saves a setting and returns the options as they are now.
	if err := w.Bind("vSetOption", func(key, id string) ([]uiOption, error) {
		s, _ := loadSettings()
		err := setUIOption(&s, key, id)
		return uiOptions(s), err
	}); err != nil {
		return err
	}
	if err := w.Bind("vGit", func() uiGit { return readUIGit(wd.get()) }); err != nil {
		return err
	}
	if err := w.Bind("vBranches", func() []string { return uiBranches(wd.get()) }); err != nil {
		return err
	}
	if err := w.Bind("vSwitch", func(name string) (uiGit, error) { return switchUIBranch(wd.get(), name) }); err != nil {
		return err
	}
	if err := w.Bind("vList", func(kind, query string) uiPick { return uiPickList(wd.get(), kind, query) }); err != nil {
		return err
	}
	if err := w.Bind("vPreview", func(kind, value string) any {
		switch kind {
		case "log":
			return uiCommitOf(wd.get(), value)
		case "switch":
			return uiBranchOf(wd.get(), value)
		}
		return uiPickPreview(wd.get(), kind, value)
	}); err != nil {
		return err
	}
	if err := w.Bind("vImage", uiImageOf); err != nil {
		return err
	}
	if err := w.Bind("vFile", uiReadFile); err != nil {
		return err
	}
	// vSetWorkdir changes the directory and returns the start data for it.
	if err := w.Bind("vSetWorkdir", func(path string) (uiStart, error) {
		_, err := wd.set(path)
		return newUIStart(wd.get()), err
	}); err != nil {
		return err
	}
	// vSend's second argument is what the shell mode did since the last message.
	if err := w.Bind("vSend", func(text, context string) error {
		if context != "" {
			text = context + "\n\n" + text
		}
		return chat.Send(wd.get(), text)
	}); err != nil {
		return err
	}
	if err := w.Bind("vShell", func(id, command string, cols int) error { return shell.Run(wd.get(), id, command, cols) }); err != nil {
		return err
	}
	if err := w.Bind("vShellStop", shell.Stop); err != nil {
		return err
	}
	if err := w.Bind("vAnswer", chat.Answer); err != nil {
		return err
	}
	if err := w.Bind("vInterrupt", chat.Interrupt); err != nil {
		return err
	}
	if err := w.Bind("vReset", func() { chat.Reset() }); err != nil {
		return err
	}
	if err := w.Bind("vResume", func(path string) []uiMsg { return chat.Resume(wd.get(), path) }); err != nil {
		return err
	}
	prs := &uiPRs{}
	if err := w.Bind("vPR", func(arg string) (uiPRHead, error) { return prs.Load(wd.get(), arg) }); err != nil {
		return err
	}
	// The actions on the open PR; each returns the PR's new head.
	for name, fn := range map[string]any{
		"vPRReply": func(thread, body string) (uiPRHead, error) { return prs.Reply(wd.get(), thread, body) },
		"vPRComment": func(path string, start, line int, body string) (uiPRHead, error) {
			return prs.Comment(wd.get(), path, start, line, body)
		},
		"vPREdit":    func(title, body string) (uiPRHead, error) { return prs.Edit(wd.get(), title, body) },
		"vPRTalk":    func(body string, summary bool) (uiPRHead, error) { return prs.Talk(wd.get(), body, summary) },
		"vPRResolve": func(thread string, resolved bool) (uiPRHead, error) { return prs.Resolve(wd.get(), thread, resolved) },
		"vPRDelete":  func(id string, conversation bool) (uiPRHead, error) { return prs.Delete(wd.get(), id, conversation) },
		"vPRSubmit":  func(verdict int, summary string) (uiPRHead, error) { return prs.Submit(wd.get(), verdict, summary) },
	} {
		if err := w.Bind(name, fn); err != nil {
			return err
		}
	}
	if err := w.Bind("vPRChecks", func() ([]uiCheck, error) { return prs.Checks(wd.get()) }); err != nil {
		return err
	}
	// vOpenURL opens a web page (a check's log, say) in the browser.
	if err := w.Bind("vOpenURL", func(link string) error {
		if u, err := url.Parse(link); err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("not a web link: %s", link)
		}
		return exec.Command("open", link).Start()
	}); err != nil {
		return err
	}
	if err := w.Bind("vBranchPRs", func() []uiPRRef { return uiBranchPRs(wd.get()) }); err != nil {
		return err
	}
	if err := w.Bind("vPRView", prs.View); err != nil {
		return err
	}
	// vIcon(base64 PNG) sets the Dock icon: the page draws it in the theme's colors.
	if err := w.Bind("vIcon", func(b64 string) {
		if png, err := base64.StdEncoding.DecodeString(b64); err == nil {
			w.Dispatch(func() { setAppIcon(png) })
		}
	}); err != nil {
		return err
	}
	if err := w.Bind("vPalette", uiPaletteOf); err != nil {
		return err
	}
	// vChrome(dark, bg) themes the window's title bar to match the page.
	if err := w.Bind("vChrome", func(dark bool, bg string) {
		w.Dispatch(func() {
			setWindowChrome(w.Window(), dark, bg)
			// The header lines up with the window buttons: tell the page where they are.
			cy, right := windowBarMetrics(w.Window())
			w.Eval(fmt.Sprintf("window.vSetBar&&window.vSetBar(%.1f,%.1f)", cy, right))
		})
	}); err != nil {
		return err
	}
	// The header is the window's title bar: the page asks to move or zoom the window.
	if err := w.Bind("vDrag", func() { w.Dispatch(func() { dragWindow(w.Window()) }) }); err != nil {
		return err
	}
	if err := w.Bind("vZoom", func() { w.Dispatch(func() { zoomWindow(w.Window()) }) }); err != nil {
		return err
	}
	if err := w.Bind("vQuit", func() { w.Terminate() }); err != nil {
		return err
	}
	w.Navigate("http://" + ln.Addr().String())
	w.Run()
	return nil
}
