package main

import (
	"fmt"
	"strings"
)

// Comment is one inline annotation anchored to a source line range.
type Comment struct {
	ID        int    `json:"id"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Quote     string `json:"quote"`
	Body      string `json:"body"`
}

// Review is the reviewer's complete submission.
type Review struct {
	Verdict  string
	Overall  string
	Comments []Comment
}

// FormatFeedback renders the review as the markdown document that
// mdreview writes to stdout for the calling agent.
func FormatFeedback(r Review) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Review: %s\n", r.Verdict)
	if overall := strings.TrimSpace(r.Overall); overall != "" {
		fmt.Fprintf(&b, "\n## Overall\n%s\n", overall)
	}
	if len(r.Comments) > 0 {
		fmt.Fprintf(&b, "\n## Comments (%d)\n", len(r.Comments))
		for i, c := range r.Comments {
			lines := fmt.Sprintf("L%d", c.StartLine)
			if c.EndLine > c.StartLine {
				lines = fmt.Sprintf("L%d-%d", c.StartLine, c.EndLine)
			}
			fmt.Fprintf(&b, "\n### %d. %s\n", i+1, lines)
			for _, q := range strings.Split(strings.TrimSpace(c.Quote), "\n") {
				fmt.Fprintf(&b, "> %s\n", q)
			}
			fmt.Fprintf(&b, "%s\n", strings.TrimSpace(c.Body))
		}
	}
	return b.String()
}
