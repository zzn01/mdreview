package main

import (
	"strings"
	"testing"
)

func mustRender(t *testing.T, src string) string {
	t.Helper()
	html, err := Render([]byte(src))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return string(html)
}

func TestRenderAnnotatesLineRanges(t *testing.T) {
	src := "# Title\n\npara one\npara two\n\n- a\n- b\n"
	html := mustRender(t, src)
	for _, want := range []string{
		`<div class="block" data-lines="1-1"><h1>Title</h1>`,
		`data-lines="3-4"`, // the two-line paragraph
		`data-lines="6-7"`, // the list spans both items
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in:\n%s", want, html)
		}
	}
}

func TestRenderGFMTable(t *testing.T) {
	src := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	html := mustRender(t, src)
	if !strings.Contains(html, "<table") {
		t.Errorf("GFM table not rendered:\n%s", html)
	}
}

func TestRenderMermaidCodeBlock(t *testing.T) {
	src := "```mermaid\ngraph TD;\nA-->B;\n```\n"
	html := mustRender(t, src)
	if !strings.Contains(html, `language-mermaid`) {
		t.Errorf("mermaid fence lost its language class:\n%s", html)
	}
}

func TestRenderInlineCode(t *testing.T) {
	src := "this has `inline code` in it\n"
	html := mustRender(t, src)
	if !strings.Contains(html, `data-lines="1-1"`) {
		t.Errorf("inline code broke line annotations:\n%s", html)
	}
	if !strings.Contains(html, "<code>") {
		t.Errorf("inline code not rendered:\n%s", html)
	}
}

func TestRenderGFMFeatures(t *testing.T) {
	src := "- [x] completed task\n- [ ] pending task\n\n~~strikethrough~~ text\n"
	html := mustRender(t, src)
	// Verify that the list block has data-lines annotation
	if !strings.Contains(html, `data-lines=`) {
		t.Errorf("GFM features broke annotations:\n%s", html)
	}
	// Verify strikethrough is rendered
	if !strings.Contains(html, "<s>") && !strings.Contains(html, "<del>") && !strings.Contains(html, "<strike>") {
		t.Errorf("strikethrough not rendered:\n%s", html)
	}
}
