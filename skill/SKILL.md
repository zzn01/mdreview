---
name: review-plan
description: Use when a local markdown file (plan/spec/design) needs human review before proceeding - opens the file from disk in a local browser review session for inline annotation and returns the feedback
---

# Review Plan

Launch a browser review session for a markdown document and wait for
the reviewer's feedback.

1. Run `mdreview <path-to-local-file.md>` with the Bash tool and
   `run_in_background: true`, where the path is a markdown file on the
   local filesystem (typically the plan/spec just written in this
   repo). NEVER run it in the foreground: the process blocks until the
   human submits, which can take longer than any Bash timeout. If the
   user asks to review from a phone or another device, add `--tunnel`
   and give them the `remote:` URL printed on stderr instead of the
   local one.
2. Tell the user their browser has opened with the review page (the
   URL is on the command's stderr as a fallback).
3. Wait for the background task to complete — do not poll. When it
   exits, read its stdout.
4. The stdout is a markdown document starting with `# Review: APPROVE`
   or `# Review: REQUEST_CHANGES`, followed by an optional overall
   comment and inline comments quoting the document with source line
   numbers (`L42-48`).
5. On REQUEST_CHANGES: apply each comment to the document, then offer
   to run another review round. On APPROVE: proceed.
