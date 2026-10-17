package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type entry struct {
	name  string
	isDir bool
}

// Browser lists one directory; the first entry is ".." unless at the root.
type Browser struct {
	dir      string
	entries  []entry
	sel, top int
}

var (
	dirStyle    = bg.Foreground(lipgloss.Color(colBlue))
	fileStyle   = bg.Foreground(lipgloss.Color(colFg))
	hiddenStyle = bg.Foreground(lipgloss.Color(colMuted))
)

// NewBrowser lists dir with the entry named sel (if any) selected.
func NewBrowser(dir, sel string) (*Browser, error) {
	b := &Browser{}
	return b, b.cd(dir, sel)
}

// cd switches to dir and selects the entry named sel, if present.
func (b *Browser) cd(dir, sel string) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	des, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var es []entry
	if dir != "/" {
		es = append(es, entry{"..", true})
	}
	for _, de := range des {
		isDir := de.IsDir()
		if de.Type()&os.ModeSymlink != 0 {
			if fi, err := os.Stat(filepath.Join(dir, de.Name())); err == nil {
				isDir = fi.IsDir()
			}
		}
		es = append(es, entry{de.Name(), isDir})
	}
	sort.SliceStable(es, func(i, j int) bool {
		if es[i].name == ".." || es[j].name == ".." {
			return es[i].name == ".."
		}
		if es[i].isDir != es[j].isDir {
			return es[i].isDir
		}
		return strings.ToLower(es[i].name) < strings.ToLower(es[j].name)
	})
	b.dir, b.entries, b.sel, b.top = dir, es, 0, 0
	for i, e := range es {
		if e.name == sel {
			b.sel = i
		}
	}
	return nil
}

func (b *Browser) Up() error {
	return b.cd(filepath.Dir(b.dir), filepath.Base(b.dir))
}

// Enter opens the selected entry: it returns a file path, or "" after entering a directory.
func (b *Browser) Enter() (string, error) {
	if len(b.entries) == 0 {
		return "", nil
	}
	e := b.entries[b.sel]
	if e.name == ".." {
		return "", b.Up()
	}
	p := filepath.Join(b.dir, e.name)
	if e.isDir {
		return "", b.cd(p, "")
	}
	return p, nil
}

func (b *Browser) Move(d, rows int) {
	b.sel = min(max(b.sel+d, 0), max(len(b.entries)-1, 0))
	if b.sel < b.top {
		b.top = b.sel
	}
	if b.sel >= b.top+rows {
		b.top = b.sel - rows + 1
	}
}

func (b *Browser) Render(width, rows int) []string {
	b.Move(0, rows)
	out := make([]string, 0, rows)
	for i := b.top; i < len(b.entries) && len(out) < rows; i++ {
		e := b.entries[i]
		name, st := e.name, fileStyle
		switch {
		case e.isDir:
			name, st = name+"/", dirStyle
		case strings.HasPrefix(name, "."):
			st = hiddenStyle
		}
		icon := "  "
		if e.isDir {
			icon = "▸ "
		}
		if i == b.sel {
			st = st.Background(lipgloss.Color(colSel)).Bold(true)
		}
		text := runewidth.Truncate("  "+icon+name, width, "…")
		out = append(out, st.Render(text+strings.Repeat(" ", max(width-lipgloss.Width(text), 0))))
	}
	for len(out) < rows {
		out = append(out, bg.Render(strings.Repeat(" ", width)))
	}
	return out
}
