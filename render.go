package main

import (
	"bytes"
	"fmt"
	"html/template"
	"sort"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// Render converts markdown to HTML, wrapping each top-level block in a
// <div class="block" data-lines="start-end"> carrying 1-based inclusive
// source line numbers. Those attributes are the anchors the review UI
// uses to map a selection back to the source file.
func Render(source []byte) (template.HTML, error) {
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	doc := md.Parser().Parse(text.NewReader(source))
	lineOf := lineIndex(source)

	var buf bytes.Buffer
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		var block bytes.Buffer
		if err := md.Renderer().Render(&block, source, n); err != nil {
			return "", err
		}
		start, end, ok := nodeLineRange(n, lineOf)
		if !ok {
			// e.g. a thematic break has no source segments; emit unwrapped
			buf.Write(block.Bytes())
			continue
		}
		fmt.Fprintf(&buf, "<div class=\"block\" data-lines=\"%d-%d\">%s</div>\n",
			start, end, block.String())
	}
	return template.HTML(buf.String()), nil
}

// lineIndex returns a function mapping a byte offset in source to a
// 1-based line number.
func lineIndex(source []byte) func(int) int {
	starts := []int{0}
	for i, b := range source {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return func(off int) int {
		return sort.Search(len(starts), func(i int) bool { return starts[i] > off })
	}
}

// nodeLineRange finds the first and last source lines covered by n's
// subtree. Container nodes (lists, blockquotes) carry no segments
// themselves, so the whole subtree is walked. Inline nodes are skipped
// because they embed BaseInline and panic when calling Lines().
func nodeLineRange(n ast.Node, lineOf func(int) int) (start, end int, ok bool) {
	minOff, maxOff := -1, -1
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		// Skip inline nodes (Type() == ast.TypeInline)
		if c.Type() == ast.TypeInline {
			return ast.WalkContinue, nil
		}
		lines := c.Lines()
		if lines != nil {
			for i := range lines.Len() {
				seg := lines.At(i)
				if minOff == -1 || seg.Start < minOff {
					minOff = seg.Start
				}
				if seg.Stop > maxOff {
					maxOff = seg.Stop
				}
			}
		}
		return ast.WalkContinue, nil
	})
	if minOff == -1 {
		return 0, 0, false
	}
	return lineOf(minOff), lineOf(max(maxOff-1, minOff)), true
}
