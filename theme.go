package main

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/lipgloss"
)

// Theme holds the UI colors and the syntax colors of one color scheme.
type Theme struct {
	ID, Name string

	Bg, Panel, Sel, Fg, Muted, Gutter string
	Accent, Accent2, Error            string // logo gradient runs Accent → Accent2

	// Syntax colors, from the TextMate scope in the comment.
	Keyword  string // keyword.control
	Func     string // entity.name.function
	String   string // string.quoted
	Number   string // constant.numeric
	Type     string // entity.name.type
	Constant string // constant.language
	Comment  string // comment
	Operator string // keyword.operator
	Variable string // variable.other
	Builtin  string // support.function
	Tag      string // entity.name.tag
	Attr     string // entity.other.attribute-name
	Escape   string // constant.character.escape
	Added    string // markup.inserted
	Deleted  string // markup.deleted

	CommentItalic bool
}

//go:embed themes/*.json
var builtinThemes embed.FS

// themes are the built-in themes followed by the user's own from ~/.v/themes,
// sorted by name. A user theme with a built-in's file name replaces it.
var themes []*Theme

const defaultTheme = "one-dark-pro-darker"

func findTheme(id string) *Theme {
	for _, t := range themes {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// cur is the active theme; the styles below are derived from it by applyTheme.
var cur *Theme

var (
	bg, sepStyle, hintStyle, errStyle, accentStyle    lipgloss.Style
	headerStyle, titleStyle, mutedHeader, gutterStyle lipgloss.Style
	fgStyle, selStyle                                 lipgloss.Style
	syntax                                            *chroma.Style
	styleCache                                        map[chroma.TokenType]lipgloss.Style
)

func init() {
	if err := loadThemes(""); err != nil {
		panic(err) // the embedded themes are part of the build
	}
	applyTheme(findTheme(defaultTheme))
}

// loadThemes reads the embedded themes, then every *.json in userDir (if set).
// Broken user themes are skipped and reported together in the error.
func loadThemes(userDir string) error {
	byID := map[string]*Theme{}
	files, _ := fs.Glob(builtinThemes, "themes/*.json")
	for _, f := range files {
		data, err := builtinThemes.ReadFile(f)
		if err != nil {
			return err
		}
		t, err := parseVSCodeTheme(strings.TrimSuffix(path.Base(f), ".json"), data)
		if err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		byID[t.ID] = t
	}
	var errs []error
	if userDir != "" {
		files, _ := filepath.Glob(filepath.Join(userDir, "*.json"))
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err == nil {
				var t *Theme
				if t, err = parseVSCodeTheme(strings.TrimSuffix(filepath.Base(f), ".json"), data); err == nil {
					byID[t.ID] = t
					continue
				}
			}
			errs = append(errs, fmt.Errorf("theme %s: %w", filepath.Base(f), err))
		}
	}
	themes = themes[:0]
	for _, t := range byID {
		themes = append(themes, t)
	}
	sort.Slice(themes, func(i, j int) bool { return themes[i].Name < themes[j].Name })
	return errors.Join(errs...)
}

func applyTheme(t *Theme) {
	cur = t
	c := func(s string) lipgloss.Color { return lipgloss.Color(s) }
	bg = lipgloss.NewStyle().Background(c(t.Bg))
	fgStyle = bg.Foreground(c(t.Fg))
	sepStyle = bg.Foreground(c(t.Gutter))
	hintStyle = bg.Foreground(c(t.Muted))
	errStyle = bg.Foreground(c(t.Error))
	accentStyle = bg.Foreground(c(t.Accent))
	selStyle = lipgloss.NewStyle().Background(c(t.Sel)).Foreground(c(t.Fg)).Bold(true)
	headerStyle = lipgloss.NewStyle().Background(c(t.Panel)).Foreground(c(t.Fg))
	titleStyle = headerStyle.Foreground(c(t.Accent)).Bold(true)
	mutedHeader = headerStyle.Foreground(c(t.Muted))
	gutterStyle = bg.Foreground(c(t.Gutter))
	comment := t.Comment
	if t.CommentItalic {
		comment = "italic " + comment
	}
	syntax = chroma.MustNewStyle(t.ID, chroma.StyleEntries{
		chroma.Background:          t.Fg + " bg:" + t.Bg,
		chroma.Comment:             comment,
		chroma.CommentPreproc:      t.Keyword,
		chroma.Keyword:             t.Keyword,
		chroma.KeywordConstant:     t.Constant,
		chroma.KeywordType:         t.Type,
		chroma.Name:                t.Fg,
		chroma.NameVariable:        t.Variable,
		chroma.NameFunction:        t.Func,
		chroma.NameBuiltin:         t.Builtin,
		chroma.NameClass:           t.Type,
		chroma.NameNamespace:       t.Type,
		chroma.NameConstant:        t.Constant,
		chroma.NameDecorator:       t.Func,
		chroma.NameTag:             t.Tag,
		chroma.NameAttribute:       t.Attr,
		chroma.NameProperty:        t.Variable,
		chroma.LiteralString:       t.String,
		chroma.LiteralStringEscape: t.Escape,
		chroma.LiteralNumber:       t.Number,
		chroma.Operator:            t.Operator,
		chroma.Punctuation:         t.Fg,
		chroma.GenericInserted:     t.Added,
		chroma.GenericDeleted:      t.Deleted,
		chroma.GenericHeading:      "bold " + t.Func,
		chroma.GenericSubheading:   t.Func,
		chroma.GenericEmph:         "italic",
		chroma.GenericStrong:       "bold",
	})
	styleCache = map[chroma.TokenType]lipgloss.Style{}
}

// styleFor maps a chroma token to a terminal style in the active theme.
func styleFor(t chroma.TokenType) lipgloss.Style {
	if s, ok := styleCache[t]; ok {
		return s
	}
	e := syntax.Get(t)
	s := fgStyle
	if e.Colour.IsSet() {
		s = s.Foreground(lipgloss.Color(e.Colour.String()))
	}
	if e.Bold == chroma.Yes {
		s = s.Bold(true)
	}
	if e.Italic == chroma.Yes {
		s = s.Italic(true)
	}
	styleCache[t] = s
	return s
}
