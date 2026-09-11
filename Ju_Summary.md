# Agentic Coding for Bioinformaticians — Workshop Summary

*3.5 hours. Nominally: build `mytools`, a small bedtools clone. Actually: learn to
specify, parallelise, review and guard work produced faster than you can read it.*

---

### 1. The subject is supervision, not the code

The BED/interval domain was chosen because it's small, well-understood, and has a
reference implementation sitting on the VM. That deliberately removes correctness as a
source of anxiety, freeing attention for the real question: how do you stay in control
of an agent that outpaces your reading speed?

### 2. The oracle principle

Real `bedtools` is installed, so every correctness question has an external arbiter. If
your output differs, *you* are wrong. This converts "is this code right?" — a question
requiring you to understand the code — into "do these bytes match?", which requires
nothing.

### 3. Verify by diffing, not by reading

The single technique the whole day rests on. You can prove a program correct without
being able to read it. That's what makes "implement it in a language you don't know" a
safe exercise rather than a reckless one.

### 4. Trust the artifact, not the agent's narration

Every exercise block ends with a **"Done when"** gate you run yourself in a plain shell.
*"The agent said it did"* and *"it is done"* are different claims, and the gap between
them is where agentic work goes wrong quietly. Closing an issue means checking the code,
not remembering that you wrote it.

### 5. Saying what you want is the skill

Nearly all workshop material is phrased as prompts to edit, not shell to copy. Typing
was never the bottleneck. Corollaries taught throughout: **give a goal, not a procedure**
("make the golden tests pass" beats a numbered list of edits — if you're writing the
steps, you're doing the work twice), and **state the workflow you want**, since an agent
left alone will branch and open a PR for a one-line fix.

### 6. Persistent context beats repetition

`CLAUDE.md` is read at the start of every session and survives `/clear`. One line there
outperforms repeating yourself across thirty prompts. The `#` prefix appends to it
mid-session. Treated as the highest-leverage file in the repo — conventions, gotchas,
hard-won rules — and as a living document you edit as the design firms up.

### 7. The warm-up is the whole day in miniature

Write a trivial program in a language you know, then the same program in one you don't,
then diff their outputs. The bottom of *99 Bottles* — a plural that stops being plural, a
count ending in a word — is structurally the same edge-case cluster as half-open interval
boundaries. Then deliberately plant an off-by-one to see that a diff tells you precisely
*that* something broke and almost nothing about *where*.

### 8. Decide the shared constraints before fanning out

An explicit "**CHOOSE YOUR LANGUAGE — stop here and decide**" beat, because five parallel
agents will each pick their own otherwise. The framing is honest about the trade: use a
familiar language if supervision is the new skill you're practising; use an unfamiliar
one if you want to test the workshop's actual claim. Either answer is fine; not answering
is not.

### 9. Specification as an interview

Rather than writing a spec from a blank page, have the agent interview you one question
at a time. The inverse works too: ramble everything you half-know into one unstructured
dump, then ask it to play back a structured list, flag your contradictions, and interview
you on the gaps. Getting it out of your head badly and having it shaped beats staring at
an empty file.

### 10. Ration your attention across decisions

Explicit permission not to care: *"do it like bedtools"*, *"the simplest thing that
works"*, *"whatever you think"* are complete answers and usually the right ones. Spend
judgement on the three or four decisions you genuinely have an opinion about. An
interview you don't finish is worth nothing, and there's a stall-time fallback spec
precisely because the spec is not the exercise.

### 11. The spec is the coordination medium

Its real job is answering every question several independent agents are about to ask.
Hence the diagnostic rule: **when two agents disagree about something, that's the spec's
fault, not theirs** — fix the spec, then tell them both. Issues decompose the spec into
independently implementable units, and become the shared surface between you and every
agent you run.

### 12. Model choice as a deliberate allocation

Plan and argue with the stronger reasoning model; build with the workhorse. Both halves
matter — a reasoning model grinding through boilerplate is money burned, and a workhorse
settling a subtle design question is a plan you rewrite an hour later. Notably, the
switch is made **before** decomposing the spec into issues, on the grounds that planning
is the highest-leverage thinking of the day.

### 13. Parallelism through crude isolation

Agents collide if they share a directory, so: one plain copy of the repo and one session
per task, tiled in `tmux`. Deliberately unclever — `git worktree` is more elegant and
explicitly deferred to *after* the workshop, because the lesson is the supervision
pattern, not the tooling. The copies are disposable; the work is on GitHub. `tmux` also
decouples session lifetime from your SSH connection, which is what makes walking away
safe.

### 14. Supervision has its own mechanics

The binding constraint shifts from typing to attention. Practical discipline: zoom into
one pane at a time (three scrolling logs is noise, not information); let plan mode finish
before approving; keep issues touching separate files, since parallelism is only free
while agents don't collide; **interrupt early**, because a wrong turn caught in ten
seconds costs ten seconds and the same turn caught in three minutes costs a checkout.

### 15. Work continues while you don't

The break is a designed exercise, not a pause: hand the agent a task longer than the
break, then leave. Remote Control moves the conversation to your phone while code,
filesystem and execution stay on the VM. The point being demonstrated is that your
presence at the keyboard is no longer what gates progress.

### 16. Two kinds of test, and the habit that grows a suite

Golden tests prove agreement with reality and catch what you never thought to check, but
need the oracle present and only tell you *that* you disagree. Unit tests pin one
behaviour each, run anywhere, and name the failure. The generative rule: **when a golden
test fails and you fix it, add the unit test that would have caught it** — that single
habit makes a suite grow in the right direction instead of merely growing. And where the
oracle surprises you, encode *its* behaviour with a comment explaining why, never what
you think it should do.

### 17. Guardrails, and testing the guardrail

The destination is CI running golden tests, unit tests and a linter on every push. Then
you plant a subtle bug on purpose. Red CI means the guardrail works; **green is the more
valuable result**, because it means your suite has a hole you now know about. Pairing up
to review a partner's PR adds the companion warning: rubber-stamping an agent's review of
an agent's code is how the whole structure collapses.

### 18. Blast radius is a first-class concern

A recurring theme rather than a single exercise. The `--repo` rule exists because `gh`
with a missing flag doesn't fail — it silently resolves to the git remote and exits 0, so
a forgotten flag is indistinguishable from success. "Show me the commands first" is the
standing habit for anything touching the network. The closing note distinguishes
containing the **filesystem** (a separate user account, a devcontainer) from containing
the **authority** — whatever you grant the agent, it has, so scope the token, not just
the home directory. The workshop VM was disposable; your laptop, sitting next to your SSH
keys and cloud credentials, is not.

### 19. Know what it costs

A wrap-up `/usage` check, deliberately placed next to what you actually built.
Distinguishes per-conversation spend from the account's running total, and frames cost
awareness as the habit that makes deliberate model-switching stick — and as how you see a
limit approaching rather than hitting it.

---

That's the workshop's argument end to end: **an external oracle makes verification cheap,
cheap verification makes review-by-diff possible, review-by-diff removes reading speed as
the constraint, and what's left — specification, decomposition, attention, and blast
radius — is the actual job.**
