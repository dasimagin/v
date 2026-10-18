package main

import (
	"strings"
	"testing"
)

func doc(t *testing.T, src string) *Doc {
	t.Helper()
	d, err := newDoc("x.go", src)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestEditing(t *testing.T) {
	d := doc(t, "func f() {\n\treturn\n}\n")
	d.Ed.Cur = pos{1, 7} // after "\treturn"
	d.Insert(" 42")
	d.Newline()
	d.Insert("x")
	if got := d.Contents(); got != "func f() {\n\treturn 42\n\tx\n}\n" {
		t.Fatalf("after typing:\n%q", got)
	}
	d.Backspace()
	d.Backspace()
	d.Backspace() // joins with the previous line
	if got := d.Contents(); got != "func f() {\n\treturn 42\n}\n" {
		t.Fatalf("after backspace:\n%q", got)
	}
	d.Ed.Cur = pos{0, 0}
	d.Delete()
	if got := d.Text[0]; got != "unc f() {" {
		t.Errorf("after delete: %q", got)
	}
	for range 10 {
		d.Undo()
	}
	if got := d.Contents(); got != "func f() {\n\treturn\n}\n" {
		t.Fatalf("after undo:\n%q", got)
	}
	d.Redo()
	if !strings.Contains(d.Contents(), "return 42") {
		t.Errorf("redo lost the typing: %q", d.Contents())
	}
}

func TestPasteAndLineEndings(t *testing.T) {
	d := doc(t, "a\r\nb")
	if !d.CRLF || d.FinalNL {
		t.Fatalf("CRLF=%v FinalNL=%v", d.CRLF, d.FinalNL)
	}
	d.Ed.Cur = pos{0, 1}
	d.Insert("1\n2\n3")
	if got := d.Contents(); got != "a1\r\n2\r\n3\r\nb" {
		t.Errorf("paste: %q", got)
	}
	if d.Ed.Cur != (pos{2, 1}) {
		t.Errorf("cursor after paste = %+v", d.Ed.Cur)
	}
}

func TestMoveKeepsColumnAcrossTabs(t *testing.T) {
	d := doc(t, "\tabc\nabcdefgh\n")
	d.Ed.Cur = pos{0, 2} // after "\ta": display column 5
	d.keepCol()
	d.Move(1, 0)
	if d.Ed.Cur != (pos{1, 5}) {
		t.Errorf("cursor = %+v, want line 1 col 5", d.Ed.Cur)
	}
	d.Move(0, 10) // wraps to the next line... there is none, so it stops at the end
	if d.Ed.Cur != (pos{1, 8}) {
		t.Errorf("cursor = %+v", d.Ed.Cur)
	}
}

func TestIndentUnit(t *testing.T) {
	if u := doc(t, "x\n    y\n").indentUnit(); u != "    " {
		t.Errorf("spaces file: %q", u)
	}
	if u := doc(t, "x\n\ty\n").indentUnit(); u != "\t" {
		t.Errorf("tabs file: %q", u)
	}
}
