package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

// mdRenderer renders the agent's markdown replies in the colors of the theme.
// It is rebuilt when the theme or the width changes.
type mdRenderer struct {
	theme *Theme
	width int
	r     *glamour.TermRenderer
}

var md mdRenderer

// markdown renders text as terminal lines of at most width cells, keeping the
// theme background across glamour's style resets.
func markdown(text string, width int) []string {
	if md.r == nil || md.theme != cur || md.width != width {
		r, err := glamour.NewTermRenderer(
			glamour.WithStyles(mdStyle(cur)),
			glamour.WithWordWrap(width),
			glamour.WithColorProfile(lipgloss.ColorProfile()),
			glamour.WithChromaFormatter("terminal16m"), // code in exact theme colors
		)
		if err != nil {
			return strings.Split(text, "\n")
		}
		md = mdRenderer{cur, width, r}
	}
	// Tabs would move the terminal cursor past our padding; expand them.
	out, err := md.r.Render(strings.ReplaceAll(text, "\t", "    "))
	if err != nil {
		return strings.Split(text, "\n")
	}
	keepBg := "\x1b[0m" + bgSeq(cur.Bg)
	out = strings.ReplaceAll(out, "\x1b[0m", keepBg)
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = bgSeq(cur.Bg) + strings.TrimRight(xansi.Truncate(l, width, ""), " ")
	}
	// Drop the blank lines glamour puts around the document.
	for len(lines) > 0 && xansi.Strip(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && xansi.Strip(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// markdownOn renders markdown on a block of the panel color: every line is
// padded to width, so the tint forms a solid rectangle.
func markdownOn(text string, width int) []string {
	lines := markdown(text, width)
	plain, tint := bgSeq(cur.Bg), bgSeq(cur.Panel)
	for i, l := range lines {
		l = strings.ReplaceAll(l, plain, tint)
		lines[i] = l + bgSeq(cur.Panel) + strings.Repeat(" ", max(width-xansi.StringWidth(l), 0)) + "\x1b[0m"
	}
	return lines
}

// bgSeq is the escape sequence that sets a "#rrggbb" background.
func bgSeq(hex string) string {
	v, _ := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", v>>16&0xff, v>>8&0xff, v&0xff)
}

// mdStyle is glamour's dark style recolored with the theme.
func mdStyle(t *Theme) ansi.StyleConfig {
	s := styles.DarkStyleConfig
	p := func(c string) *string { return &c }
	b := func(v bool) *bool { return &v }
	zero := uint(0)
	col := func(c string) ansi.StylePrimitive { return ansi.StylePrimitive{Color: p(c)} }

	s.Document = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: p(t.Fg)}, Margin: &zero}
	s.Paragraph = ansi.StyleBlock{}
	s.Heading = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{BlockSuffix: "\n", Color: p(t.Accent), Bold: b(true)}}
	// Headings are colored instead of keeping their "#" marks.
	s.H1 = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: p(t.Accent2), Bold: b(true)}}
	s.H2, s.H3, s.H4, s.H5 = ansi.StyleBlock{}, ansi.StyleBlock{}, ansi.StyleBlock{}, ansi.StyleBlock{}
	s.H6 = ansi.StyleBlock{StylePrimitive: col(t.Muted)}
	s.BlockQuote = ansi.StyleBlock{StylePrimitive: col(t.Muted), Indent: uintPtr(1), IndentToken: p("│ ")}
	s.HorizontalRule = ansi.StylePrimitive{Color: p(t.Gutter), Format: "\n────────\n"}
	s.Link = ansi.StylePrimitive{Color: p(t.Accent), Underline: b(true)}
	s.LinkText = ansi.StylePrimitive{Color: p(t.Accent), Bold: b(true)}
	s.Code = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: p(t.Variable), BackgroundColor: p(t.Sel)}}
	s.CodeBlock = ansi.StyleCodeBlock{
		StyleBlock: ansi.StyleBlock{StylePrimitive: col(t.Fg), Margin: uintPtr(2)},
		Chroma: &ansi.Chroma{
			Text:                col(t.Fg),
			Error:               col(t.Error),
			Comment:             ansi.StylePrimitive{Color: p(t.Comment), Italic: b(t.CommentItalic)},
			CommentPreproc:      col(t.Keyword),
			Keyword:             col(t.Keyword),
			KeywordReserved:     col(t.Keyword),
			KeywordNamespace:    col(t.Keyword),
			KeywordType:         col(t.Type),
			Operator:            col(t.Operator),
			Punctuation:         col(t.Fg),
			Name:                col(t.Fg),
			NameBuiltin:         col(t.Builtin),
			NameTag:             col(t.Tag),
			NameAttribute:       col(t.Attr),
			NameClass:           col(t.Type),
			NameConstant:        col(t.Constant),
			NameDecorator:       col(t.Func),
			NameFunction:        col(t.Func),
			LiteralNumber:       col(t.Number),
			LiteralString:       col(t.String),
			LiteralStringEscape: col(t.Escape),
			GenericDeleted:      col(t.Deleted),
			GenericInserted:     col(t.Added),
			GenericEmph:         ansi.StylePrimitive{Italic: b(true)},
			GenericStrong:       ansi.StylePrimitive{Bold: b(true)},
			GenericSubheading:   col(t.Muted),
			Background:          ansi.StylePrimitive{BackgroundColor: p(t.Bg)},
		},
	}
	return s
}

func uintPtr(u uint) *uint { return &u }
