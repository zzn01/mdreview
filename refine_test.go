package main

import "testing"

func TestRefineLinesSingleLineMatch(t *testing.T) {
	lines := []string{
		"line one",
		"line two",
		"target text here",
		"line four",
		"line five",
	}
	s, e := refineLines(lines, 1, 5, "target text here")
	if s != 3 || e != 3 {
		t.Fatalf("got (%d,%d), want (3,3)", s, e)
	}
}

func TestRefineLinesNoMatchReturnsOriginalBounds(t *testing.T) {
	lines := []string{"one", "two", "three", "four", "five"}
	s, e := refineLines(lines, 2, 4, "nonexistent phrase")
	if s != 2 || e != 4 {
		t.Fatalf("got (%d,%d), want (2,4)", s, e)
	}
}

func TestRefineLinesMultiLineMatchesFirstAndLast(t *testing.T) {
	lines := []string{
		"Block header",
		"first line of quote",
		"middle irrelevant",
		"last line of quote",
		"trailing",
	}
	s, e := refineLines(lines, 1, 5, "first line of quote\nlast line of quote")
	if s != 2 || e != 4 {
		t.Fatalf("got (%d,%d), want (2,4)", s, e)
	}
}

func TestRefineLinesMultiLineFirstFoundLastUnfound(t *testing.T) {
	lines := []string{
		"Header",
		"first line of quote",
		"irrelevant one",
		"irrelevant two",
		"irrelevant three",
	}
	// last line of the quote doesn't appear anywhere in the block; end
	// falls back to start-of-match + quote length, clamped to the block.
	s, e := refineLines(lines, 1, 5, "first line of quote\nlast line not present")
	if s != 2 || e != 3 {
		t.Fatalf("got (%d,%d), want (2,3)", s, e)
	}
}

func TestRefineLinesMultiLineFirstUnfoundLastFound(t *testing.T) {
	lines := []string{
		"Header",
		"irrelevant one",
		"irrelevant two",
		"last line of quote",
		"trailing",
	}
	s, e := refineLines(lines, 1, 5, "missing first line\nlast line of quote")
	if s != 1 || e != 4 {
		t.Fatalf("got (%d,%d), want (1,4)", s, e)
	}
}

func TestRefineLinesSoftWrappedParagraph(t *testing.T) {
	// The browser joins a two-line selection into one space-separated
	// string with no embedded newline, so this is a single-line quote
	// with no exact match; word fragments must recover both lines.
	lines := []string{"para one", "para two"}
	s, e := refineLines(lines, 1, 2, "para one para two")
	if s != 1 || e != 2 {
		t.Fatalf("got (%d,%d), want (1,2)", s, e)
	}
}

func TestRefineLinesDuplicateOccurrenceFirstWins(t *testing.T) {
	lines := []string{"dup line", "other", "dup line"}
	s, e := refineLines(lines, 1, 3, "dup line")
	if s != 1 || e != 1 {
		t.Fatalf("got (%d,%d), want (1,1)", s, e)
	}
}

func TestRefineLinesInlineMarkupFallback(t *testing.T) {
	lines := []string{
		"Intro line",
		"improve **Sin** precision for math",
		"Trailing line",
	}
	s, e := refineLines(lines, 1, 3, "improve Sin precision")
	if s != 2 || e != 2 {
		t.Fatalf("got (%d,%d), want (2,2)", s, e)
	}
}

func TestRefineLinesClampsBounds(t *testing.T) {
	lines := []string{"one", "two", "three"}
	s, e := refineLines(lines, -5, 100, "")
	if s != 1 || e != 3 {
		t.Fatalf("got (%d,%d), want (1,3)", s, e)
	}
}
