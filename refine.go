package main

import "strings"

// refineLines narrows a block-granular line range [start, end] (1-based,
// inclusive) to the source lines that actually contain the quoted
// selection. Markdown inline syntax can defeat exact matching; every
// failure falls back to the wider given bounds, never outside them.
func refineLines(lines []string, start, end int, quote string) (int, int) {
	if start < 1 {
		start = 1
	}
	if start > len(lines) {
		start = len(lines)
	}
	if end < 1 {
		end = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	quote = strings.TrimSpace(quote)
	if start > end || quote == "" {
		return start, end
	}

	qls := quoteLines(quote)
	if len(qls) == 0 {
		return start, end
	}
	if len(qls) > 1 {
		return refineMultiLine(lines, start, end, qls)
	}
	return refineSingleLine(lines, start, end, qls[0])
}

// refineMultiLine handles a quote spanning several lines: the first and
// last quote lines are located independently, each falling back to its
// original bound when not found.
func refineMultiLine(lines []string, start, end int, qls []string) (int, int) {
	sRaw := findLine(lines, start, end, qls[0])
	eRaw := findLineLast(lines, max(sRaw, start), end, qls[len(qls)-1])

	s := start
	if sRaw != 0 {
		s = sRaw
	}
	e := end
	if eRaw != 0 {
		e = eRaw
	} else if sRaw != 0 {
		e = min(s+len(qls)-1, end)
	}
	return s, e
}

// refineSingleLine handles a one-line quote. An exact substring match
// always wins and pins a single line (first occurrence, even if the
// quote recurs elsewhere in the block). Without an exact match — e.g. a
// soft-wrapped selection or inline markup breaking the substring — word
// fragments from the start and end of the quote are used instead.
func refineSingleLine(lines []string, start, end int, q string) (int, int) {
	if s0 := findLine(lines, start, end, q); s0 != 0 {
		return s0, s0
	}

	startFrag := firstWordFrag(q)
	endFrag := lastWordFrag(q)
	sRaw := findLine(lines, start, end, startFrag)
	eRaw := findLineLast(lines, max(sRaw, start), end, endFrag)

	s := start
	if sRaw != 0 {
		s = sRaw
	}
	e := end
	if eRaw != 0 {
		e = eRaw
	}
	if s > e {
		return start, end
	}
	return s, e
}

// findLine returns the first line number in [from, to] (1-based,
// inclusive) whose content contains frag, or 0 if none does.
func findLine(lines []string, from, to int, frag string) int {
	for i := from; i <= to; i++ {
		if strings.Contains(lines[i-1], frag) {
			return i
		}
	}
	return 0
}

// findLineLast returns the last line number in [from, to] (1-based,
// inclusive) whose content contains frag, or 0 if none does.
func findLineLast(lines []string, from, to int, frag string) int {
	for i := to; i >= from; i-- {
		if strings.Contains(lines[i-1], frag) {
			return i
		}
	}
	return 0
}

// quoteLines splits a quote into its trimmed, non-empty lines.
func quoteLines(quote string) []string {
	var out []string
	for _, l := range strings.Split(quote, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// firstWordFrag returns the first word of s with length >= 4, or the
// first word if none qualifies.
func firstWordFrag(s string) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return s
	}
	for _, w := range words {
		if len(w) >= 4 {
			return w
		}
	}
	return words[0]
}

// lastWordFrag returns the last word of s with length >= 4, or the last
// word if none qualifies.
func lastWordFrag(s string) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return s
	}
	for i := len(words) - 1; i >= 0; i-- {
		if len(words[i]) >= 4 {
			return words[i]
		}
	}
	return words[len(words)-1]
}
