package main

import "testing"

func TestFormatFeedbackApproveNoComments(t *testing.T) {
	got := FormatFeedback(Review{Verdict: "APPROVE"})
	want := "# Review: APPROVE\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormatFeedbackFull(t *testing.T) {
	got := FormatFeedback(Review{
		Verdict: "REQUEST_CHANGES",
		Overall: "tighten the scope\n",
		Comments: []Comment{
			{StartLine: 42, EndLine: 48, Quote: "first line\nsecond line", Body: "fix the race"},
			{StartLine: 60, EndLine: 60, Quote: "misc", Body: "rename this section"},
		},
	})
	want := `# Review: REQUEST_CHANGES

## Overall
tighten the scope

## Comments (2)

### 1. L42-48
> first line
> second line
fix the race

### 2. L60
> misc
rename this section
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatFeedbackOmitsEmptyOverall(t *testing.T) {
	got := FormatFeedback(Review{
		Verdict:  "REQUEST_CHANGES",
		Overall:  "  \n",
		Comments: []Comment{{StartLine: 1, EndLine: 1, Quote: "q", Body: "b"}},
	})
	want := `# Review: REQUEST_CHANGES

## Comments (1)

### 1. L1
> q
b
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}
