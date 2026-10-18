package main

import (
	"strings"
	"testing"
)

func docFrom(base, cur string) *Doc {
	_, bl, _ := highlight("x.txt", base)
	_, cl, _ := highlight("x.txt", cur)
	return &Doc{Lines: cl, Text: texts(cl), Base: &Base{Lines: bl, Text: texts(bl)}}
}

func kinds(rows []diffRow, right bool) string {
	var b strings.Builder
	for _, r := range rows {
		s := r.left
		if right {
			s = r.right
		}
		b.WriteByte("caxh."[s.kind])
	}
	return b.String()
}

func TestUnifiedRows(t *testing.T) {
	d := docFrom("a\nb\nc\nd\n", "a\nB\nc\nd\ne\n")
	rows := unifiedRows(d)
	// hunk, a, -b, +B, c, d, +e
	if got := kinds(rows, false); got != "hcxacca" {
		t.Fatalf("kinds = %q", got)
	}
	if rows[0].head != "@@ -1,4 +1,5 @@" {
		t.Errorf("head = %q", rows[0].head)
	}
	if r := rows[2]; r.oldNo != 2 || texts([][]seg{r.left.segs})[0] != "b" {
		t.Errorf("deleted row = %+v", r)
	}
	if r := rows[6]; r.left.no != 5 || texts([][]seg{r.left.segs})[0] != "e" {
		t.Errorf("added row = %+v", r)
	}
}

func TestSplitRows(t *testing.T) {
	d := docFrom("a\nb\nc\n", "a\nB\nB2\nc\n")
	rows := splitRows(d)
	if got := kinds(rows, false); got != "hcx.c" {
		t.Errorf("left kinds = %q", got)
	}
	if got := kinds(rows, true); got != "hcaac" {
		t.Errorf("right kinds = %q", got)
	}
}

func TestNoChangesAndNewFile(t *testing.T) {
	if rows := unifiedRows(docFrom("a\n", "a\n")); len(rows) != 0 {
		t.Errorf("same text gave %d rows", len(rows))
	}
	d := docFrom("", "x\ny\n")
	d.Base = &Base{New: true}
	if got := kinds(unifiedRows(d), false); got != "haa" {
		t.Errorf("new file kinds = %q", got)
	}
}
