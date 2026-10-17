package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/mattn/go-runewidth"
)

// seg is a run of text of one token type; its style comes from the active theme.
type seg struct {
	text string
	tok  chroma.TokenType
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
	lang, lines, err := highlight(path, string(b))
	if err != nil {
		return nil, err
	}
	return &Doc{Path: path, Lang: lang, Lines: lines}, nil
}

// highlight splits src into lines of tokens. The lexer is picked by file
// name, then by content; it returns the language name.
func highlight(name, src string) (string, [][]seg, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\t", "    ")

	lexer := lexers.Match(name)
	if lexer == nil {
		lexer = lexers.Analyse(src)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)
	it, err := lexer.Tokenise(nil, src)
	if err != nil {
		return "", nil, err
	}

	lines := [][]seg{nil}
	for tok := it(); tok != chroma.EOF; tok = it() {
		for i, part := range strings.Split(tok.Value, "\n") {
			if i > 0 {
				lines = append(lines, nil)
			}
			if part != "" {
				lines[len(lines)-1] = append(lines[len(lines)-1], seg{part, tok.Type})
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
	return lang, lines, nil
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
			sb.WriteString(styleFor(s.tok).Render(chunk.String()))
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
