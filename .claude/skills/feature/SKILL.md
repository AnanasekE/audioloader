---
name: feature
description: Plan a feature from REQUESTS.md on Opus, then implement it with Sonnet subagents. Use when the user runs /feature, optionally with a request name.
argument-hint: "[request name or short description]"
---

# Feature: plan on Opus, implement on Sonnet

The main session (Opus) reads, plans, reviews and verifies. All code is written by Sonnet subagents (`model: "sonnet"`). Keep chat output short.

## 1. Pick the request

- Read `REQUESTS.md`, `FEATURES.md` and `docs/plans/README.md`.
- If `$ARGUMENTS` names a request, use it. Otherwise list the open requests in one line each and ask the user which one (AskUserQuestion), recommending the smallest self-contained one.
- If the request has **Open** questions that change the design, ask them before planning.

## 2. Plan (Opus, this session)

- Read the code the feature touches (handlers in `internal/api`, SQL in `internal/db`, templ in `internal/components`, JS in `assets/js`) and the closest earlier plan for style.
- Write `docs/plans/NN-<slug>.md`, NN = next free number, in the format of the existing plans:
  - Title, **Goal**, **Depends on**.
  - **Status**: a `- [ ]` checklist, one item per step.
  - **Rules**: `make build`, `go vet ./...` and `make test` pass; one commit per step; app runnable after each step; no attribution lines in commits; plain JS in `assets/js`, no build step; pure logic in small node-tested modules; tests use `httptest` fakes, never real services; never run anything on port 8080 (the user's server).
  - **Decisions**: concrete choices (schema, routes, names, UI placement) so the implementer doesn't have to decide.
  - **Steps**: per step, the files to change and what to test.
- Add the plan to the table in `docs/plans/README.md`.
- Commit the plan (`Plan NN: <title>`), no attribution lines.
- Show the user a 3–5 line summary and ask whether to implement.

## 3. Implement (Sonnet subagents)

- Spawn one `general-purpose` agent with `model: "sonnet"` per plan (or per step if the plan is large). Give it: the plan path, "do steps X–Y in order, one commit each, tick the Status box in the same commit", the Rules above, and "report what you changed and any test output that failed".
- Don't write feature code in the main session. If an agent fails, fix the plan or the prompt and send it back (SendMessage), don't take over.

## 4. Verify (Opus)

- Run `make build`, `go vet ./...`, `make test`.
- Review the diff (`git log`, `git diff main...`) against the plan's Decisions.
- For UI changes: run the app on a free port other than 8080 and take headless Firefox screenshots at 1440×900 and 390×844; look at them.
- When done: move the request out of `REQUESTS.md`, add it to `FEATURES.md`, mark the plan implemented in `docs/plans/README.md`, commit.
- Report to the user in a few lines: what was built, test results, anything skipped.
