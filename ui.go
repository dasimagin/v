package main

import (
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"sync"
)

// uiCommand is an omnibox command as the UI's suggestions show it.
type uiCommand struct {
	Name      string   `json:"name"`
	Aliases   []string `json:"aliases"`
	Args      string   `json:"args"`
	Desc      string   `json:"desc"`
	Supported bool     `json:"supported"` // has a screen in the graphical UI yet
}

// uiStart is everything the start screen needs.
type uiStart struct {
	Workdir  string      `json:"workdir"`
	Tagline  string      `json:"tagline"`
	Palette  uiPalette   `json:"palette"`
	Animate  bool        `json:"animate"`
	Git      uiGit       `json:"git"`
	Commands []uiCommand `json:"commands"`
}

// uiGit is the state of the working directory's repository for the status bar.
type uiGit struct {
	Branch string `json:"branch"` // "" outside a repository
	Dirty  int    `json:"dirty"`  // changed files
}

func readUIGit(dir string) uiGit {
	branch, err := gitOutput(dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		if branch, err = gitOutput(dir, "rev-parse", "--short", "HEAD"); err != nil {
			return uiGit{}
		}
		branch = "(" + branch + ")"
	}
	out, _ := gitOutput(dir, "status", "--porcelain")
	g := uiGit{Branch: branch}
	if out = strings.TrimSpace(out); out != "" {
		g.Dirty = strings.Count(out, "\n") + 1
	}
	return g
}

// uiBranches are the local branches of dir's repository, current first.
func uiBranches(dir string) []string {
	r := readRepository(dir)
	if r == nil {
		return []string{}
	}
	out := []string{}
	if r.Branch != "" {
		out = append(out, r.Branch)
	}
	for _, b := range r.Branches {
		if !b.Remote && b.Name != r.Branch {
			out = append(out, b.Name)
		}
	}
	return out
}

// switchUIBranch checks a branch out and returns the repository's new state;
// Git's message is the error when it refuses (say, over local changes).
func switchUIBranch(dir, name string) (uiGit, error) {
	_, err := gitOutput(dir, "switch", name)
	return readUIGit(dir), err
}

// uiLight is the one theme made for the graphical UI only: the terminal themes
// are all dark.
var uiLight = &Theme{ID: "light", Name: "Light", Bg: "#f3f4f6", Panel: "#fafafa", Fg: "#1f2430", Muted: "#8a909c", Accent: "#3f729b", Accent2: "#7a55b8",
	Keyword: "#a626a4", Func: "#4078f2", String: "#50a14f", Number: "#986801", Type: "#c18401", Constant: "#986801",
	Comment: "#a0a1a7", Operator: "#0184bc", Variable: "#e45649", Builtin: "#0184bc", Tag: "#e45649", Attr: "#986801",
	Escape: "#0184bc", Added: "#2e8b3d", Deleted: "#cb3a3a"}

// uiThemes are the terminal themes and the light one.
func uiThemes() []*Theme { return append(slices.Clone(themes), uiLight) }

// uiPalette is the few colors the page derives all of its own from.
type uiPalette struct {
	Bg      string `json:"bg"`
	Panel   string `json:"panel"`
	Fg      string `json:"fg"`
	Muted   string `json:"muted"`
	Accent  string `json:"accent"`
	Accent2 string `json:"accent2"` // the brand gradient runs accent → accent2
	Dark    bool   `json:"dark"`
	// Syntax colors by token class (see tokenClass).
	Syntax map[string]string `json:"syntax"`
}

func uiPaletteOf(id string) uiPalette {
	t := uiLight
	if id != uiLight.ID {
		if t = findTheme(id); t == nil {
			t = findTheme(defaultTheme)
		}
	}
	return uiPalette{t.Bg, t.Panel, t.Fg, t.Muted, t.Accent, t.Accent2, !isLight(t.Bg), map[string]string{
		"keyword": t.Keyword, "func": t.Func, "string": t.String, "number": t.Number, "type": t.Type,
		"constant": t.Constant, "comment": t.Comment, "operator": t.Operator, "variable": t.Variable,
		"builtin": t.Builtin, "tag": t.Tag, "attr": t.Attr, "escape": t.Escape, "added": t.Added, "deleted": t.Deleted,
	}}
}

// uiSupported are the commands the graphical UI can run so far.
var uiSupported = []string{"config", "open", "show", "workdir", "switch", "resume", "pr", "reset", "quit"}

func newUIStart(dir string) uiStart {
	s, _ := loadSettings()
	st := uiStart{Workdir: dir, Tagline: randomTagline(), Palette: uiPaletteOf(s.UITheme), Animate: s.Animate, Git: readUIGit(dir)}
	for _, c := range commands {
		st.Commands = append(st.Commands, uiCommand{c.Name, c.Aliases, c.Args, c.Desc, slices.Contains(uiSupported, c.Name)})
	}
	return st
}

func randomTagline() string { return taglines[rand.Intn(len(taglines))] }

// uiChoice is one value of a setting.
type uiChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// uiOption is one row of the config screen.
type uiOption struct {
	Key     string     `json:"key"`
	Label   string     `json:"label"`
	Desc    string     `json:"desc"`
	Choices []uiChoice `json:"choices"`
	Current string     `json:"current"`
}

func uiOptions(s Settings) []uiOption {
	var mc, ec, tc []uiChoice
	for _, t := range uiThemes() {
		tc = append(tc, uiChoice{t.ID, t.Name})
	}
	for _, m := range models {
		mc = append(mc, uiChoice{m.ID, m.Name})
	}
	for _, e := range efforts {
		ec = append(ec, uiChoice{e, e})
	}
	manualAuto := []uiChoice{{reviewManual, "Manual"}, {reviewAuto, "Auto"}}
	return []uiOption{
		{"theme", "Color theme", "The colors of the window; the dark ones are the terminal's themes", tc, s.UITheme},
		{"model", "Model", "The model the agent runs on", mc, s.Model},
		{"effort", "Effort", "How hard the model thinks: more is slower and uses more of your plan", ec, s.Effort},
		{"review_mode", "Review mode (experimental)", "Manual asks about each comment; Auto sends them at once as a pending review you publish later", manualAuto, s.ReviewMode},
		{"apply_mode", "Apply mode (experimental)", "Manual asks about every edit, commit, push and reply; Auto lets the agent do all of it", manualAuto, s.ApplyMode},
	}
}

// setUIOption changes one setting to a value from its choices and saves it.
func setUIOption(s *Settings, key, id string) error {
	for _, o := range uiOptions(*s) {
		if o.Key != key {
			continue
		}
		if !slices.ContainsFunc(o.Choices, func(c uiChoice) bool { return c.ID == id }) {
			return fmt.Errorf("%s: unknown value %q", key, id)
		}
		switch key {
		case "theme":
			s.UITheme = id
		case "model":
			s.Model = id
		case "effort":
			s.Effort = id
		case "review_mode":
			s.ReviewMode = id
		case "apply_mode":
			s.ApplyMode = id
		}
		return s.save()
	}
	return fmt.Errorf("unknown setting %q", key)
}

// takeFlag removes every occurrence of flag from args and reports whether it was there.
func takeFlag(args []string, flag string) ([]string, bool) {
	rest := slices.DeleteFunc(slices.Clone(args), func(a string) bool { return a == flag })
	return rest, len(rest) != len(args)
}

// uiWorkdir is the directory the UI works in; /workdir changes it.
type uiWorkdir struct {
	mu  sync.RWMutex
	dir string
}

func (w *uiWorkdir) get() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.dir
}

// set changes the directory to path (absolute, or relative to the current one).
func (w *uiWorkdir) set(path string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	dir, err := workingDirectory(w.dir, path)
	if err != nil {
		return w.dir, err
	}
	w.dir = dir
	return dir, nil
}
