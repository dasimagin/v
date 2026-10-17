package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

var theme = chroma.MustNewStyle("one-dark-pro-darker", chroma.StyleEntries{
	chroma.Background:          colFg + " bg:" + colBg,
	chroma.Comment:             "italic " + colMuted,
	chroma.CommentPreproc:      colPurple,
	chroma.Keyword:             colPurple,
	chroma.KeywordConstant:     colOrange,
	chroma.KeywordType:         colYellow,
	chroma.Name:                colFg,
	chroma.NameVariable:        colRed,
	chroma.NameFunction:        colBlue,
	chroma.NameBuiltin:         colCyan,
	chroma.NameClass:           colYellow,
	chroma.NameNamespace:       colYellow,
	chroma.NameConstant:        colOrange,
	chroma.NameDecorator:       colBlue,
	chroma.NameTag:             colRed,
	chroma.NameAttribute:       colOrange,
	chroma.NameProperty:        colRed,
	chroma.LiteralString:       colGreen,
	chroma.LiteralStringEscape: colCyan,
	chroma.LiteralNumber:       colOrange,
	chroma.Operator:            colCyan,
	chroma.Punctuation:         colFg,
	chroma.GenericInserted:     colGreen,
	chroma.GenericDeleted:      colRed,
	chroma.GenericHeading:      "bold " + colBlue,
	chroma.GenericSubheading:   colBlue,
	chroma.GenericEmph:         "italic",
	chroma.GenericStrong:       "bold",
})

// seg is a run of text drawn with one style.
type seg struct {
	text  string
	style lipgloss.Style
}

type Doc struct {
	Path  string
	Lang  string
	Lines [][]seg
}

func LoadDoc(path string) (*Doc, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
		return nil, errors.New("binary file")
	}
	src := strings.ReplaceAll(string(b), "\r\n", "\n")
	src = strings.ReplaceAll(src, "\t", "    ")

	lexer := lexers.Match(path)
	if lexer == nil {
		lexer = lexers.Analyse(src)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)
	it, err := lexer.Tokenise(nil, src)
	if err != nil {
		return nil, err
	}

	lines := [][]seg{nil}
	for tok := it(); tok != chroma.EOF; tok = it() {
		st := styleFor(tok.Type)
		for i, part := range strings.Split(tok.Value, "\n") {
			if i > 0 {
				lines = append(lines, nil)
			}
			if part != "" {
				lines[len(lines)-1] = append(lines[len(lines)-1], seg{part, st})
			}
		}
	}
	if strings.HasSuffix(src, "\n") && len(lines) > 1 {
		lines = lines[:len(lines)-1]
	}
	lang := lexer.Config().Name
	if lang == "fallback" {
		lang = "plain text"
	}
	return &Doc{Path: path, Lang: lang, Lines: lines}, nil
}

var styleCache = map[chroma.TokenType]lipgloss.Style{}

func styleFor(t chroma.TokenType) lipgloss.Style {
	if s, ok := styleCache[t]; ok {
		return s
	}
	e := theme.Get(t)
	s := lipgloss.NewStyle().Background(lipgloss.Color(colBg)).Foreground(lipgloss.Color(colFg))
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

// renderLine draws columns [left, left+width) of a line, padded to width.
func renderLine(segs []seg, left, width int) string {
	var sb strings.Builder
	col, used := 0, 0
	for _, s := range segs {
		var chunk strings.Builder
		for _, r := range s.text {
			w := runewidth.RuneWidth(r)
			if col >= left && used+w <= width {
				chunk.WriteRune(r)
				used += w
			}
			col += w
		}
		if chunk.Len() > 0 {
			sb.WriteString(s.style.Render(chunk.String()))
		}
		if used >= width {
			break
		}
	}
	if used < width {
		sb.WriteString(bg.Render(strings.Repeat(" ", width-used)))
	}
	return sb.String()
}
