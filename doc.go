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

// seg is a run of text of one token type; its style comes from the active theme.
type seg struct {
	text string
	tok  chroma.TokenType
}

type Doc struct {
	Path  string
	Lang  string
	Lines [][]seg  // highlighted Text
	Text  []string // the file's lines as they are, tabs included
	Base  *Base    // HEAD version; nil outside a git repository
	// Review is the agent tool ("Edit", "Write") whose proposed result this
	// is; Base is then the file on disk. Empty for files opened by the user.
	Review string
	Orig   []string // the proposal as the agent made it, before user edits

	CRLF    bool // lines end with \r\n; kept when saving
	FinalNL bool // the file ends with a newline

	Ed Editor // cursor, undo history and the dirty flag

	unified, split []diffRow // built on first use
	unifiedAt      []int     // row of each Text line in unified, -1 if hidden
	splitAt        []int     // the same for side-by-side
}

func LoadDoc(path string) (*Doc, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
		return nil, errors.New("binary file")
	}
	d, err := newDoc(path, string(b))
	if err != nil {
		return nil, err
	}
	d.Base = loadBase(path)
	return d, nil
}

// newDoc makes a document from file contents.
func newDoc(path, src string) (*Doc, error) {
	d := &Doc{Path: path, CRLF: strings.Contains(src, "\r\n"), FinalNL: strings.HasSuffix(src, "\n")}
	src = strings.ReplaceAll(src, "\r\n", "\n")
	d.Text = strings.Split(strings.TrimSuffix(src, "\n"), "\n")
	return d, d.rehighlight()
}

// rehighlight rebuilds Lines from Text and drops the cached diffs.
func (d *Doc) rehighlight() error {
	lang, lines, err := highlight(d.Path, strings.Join(d.Text, "\n"))
	if err != nil {
		return err
	}
	d.Lang, d.Lines = lang, lines
	for len(d.Lines) < len(d.Text) {
		d.Lines = append(d.Lines, nil)
	}
	d.unified, d.split = nil, nil
	return nil
}

// Contents is the file as it would be saved.
func (d *Doc) Contents() string {
	nl := "\n"
	if d.CRLF {
		nl = "\r\n"
	}
	s := strings.Join(d.Text, nl)
	if d.FinalNL {
		s += nl
	}
	return s
}

// highlight splits src into lines of tokens. The lexer is picked by file
// name, then by content; it returns the language name.
func highlight(name, src string) (string, [][]seg, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")

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

// tabWidth is how many columns a tab stop is.
const tabWidth = 4

// runeCols is how many columns r takes at display column col.
func runeCols(r rune, col int) int {
	if r == '\t' {
		return tabWidth - col%tabWidth
	}
	return runewidth.RuneWidth(r)
}

// displayCol is the display column of rune index i in line.
func displayCol(line string, i int) int {
	col := 0
	for _, r := range []rune(line)[:min(i, utf8.RuneCountInString(line))] {
		col += runeCols(r, col)
	}
	return col
}

// runeAt is the rune index under display column col of line.
func runeAt(line string, col int) int {
	c := 0
	for i, r := range []rune(line) {
		w := runeCols(r, c)
		if col < c+w {
			return i
		}
		c += w
	}
	return utf8.RuneCountInString(line)
}

// renderLine draws columns [left, left+width) of a line, padded to width.
func renderLine(segs []seg, left, width int) string {
	return renderLineOn(segs, left, width, "", -1)
}

// renderLineOn is renderLine over a tinted background ("" for the theme's),
// with a block cursor at display column cursor (-1 for none). Tabs expand
// to the next tab stop.
func renderLineOn(segs []seg, left, width int, tint lipgloss.Color, cursor int) string {
	padStyle := bg
	if tint != "" {
		padStyle = bg.Background(tint)
	}
	var sb strings.Builder
	col, used := 0, 0
	put := func(st lipgloss.Style, text string, at int) {
		if at == cursor {
			st = st.Reverse(true)
		}
		sb.WriteString(st.Render(text))
	}
	for _, s := range segs {
		st := styleFor(s.tok)
		if tint != "" {
			st = st.Background(tint)
		}
		var chunk strings.Builder
		flush := func() {
			if chunk.Len() > 0 {
				put(st, chunk.String(), -2)
				chunk.Reset()
			}
		}
		for _, r := range s.text {
			w := runeCols(r, col)
			text := string(r)
			if r == '\t' {
				text = strings.Repeat(" ", w)
			}
			if col >= left && used+w <= width {
				if col == cursor {
					// The cursor takes the rune's first cell.
					flush()
					first, rest := text, ""
					if r == '\t' {
						first, rest = " ", strings.Repeat(" ", w-1)
					}
					put(st, first, col)
					chunk.WriteString(rest)
				} else {
					chunk.WriteString(text)
				}
				used += w
			}
			col += w
		}
		flush()
		if used >= width {
			break
		}
	}
	if used < width {
		if cursor >= col && cursor-left < width && cursor >= left {
			// The cursor is past the end of the line.
			gap := cursor - max(col, left)
			sb.WriteString(padStyle.Render(strings.Repeat(" ", gap)))
			sb.WriteString(padStyle.Reverse(true).Render(" "))
			used += gap + 1
		}
		sb.WriteString(padStyle.Render(strings.Repeat(" ", max(width-used, 0))))
	}
	return sb.String()
}
