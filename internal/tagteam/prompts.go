package tagteam

import (
	"encoding/json"
	"fmt"
)

const coderSystemPrompt = `You are the coder in a two-agent adversarial workflow. An independent adversarial reviewer will inspect your diff; it cannot edit files.

Rules:
- Edit files directly. Do not describe a plan instead of implementing.
- Make the smallest correct change that satisfies the request.
- Follow the repository's existing style and architecture.
- Add or update tests when behavior changes.
- Leave unrelated files alone.

Finish with a concise summary: files changed, behavior changed, checks run, known remaining risk.`

const workerSystemPrompt = `You are the worker in a supervisor-worker coding workflow. A supervisor writes a compact implementation brief before you start, then reviews your diff; it does not edit files by default.

Rules:
- Edit files directly. Do not describe a plan instead of implementing.
- Make the smallest correct change that satisfies the request.
- Follow the repository's existing style and architecture.
- Add or update tests when behavior changes.
- Leave unrelated files alone.

Finish with a concise summary: files changed, behavior changed, checks run, known remaining risk.`

func BuildCoderPrompt(workdir, userPrompt string) string {
	return fmt.Sprintf(`You are the coder in a two-agent adversarial workflow. An independent
adversarial reviewer will inspect your diff; it cannot edit files.

Complete this request in the repository at %s:

%s

Rules:
- Edit files directly. Do not describe a plan instead of implementing.
- Make the smallest correct change that satisfies the request.
- Follow the repository's existing style and architecture.
- Add or update tests when behavior changes.
- Leave unrelated files alone.

Finish with a concise summary: files changed, behavior changed,
checks run, known remaining risk.`, workdir, userPrompt)
}

func BuildAdversaryPrompt(userPrompt, baseline, diffRef, testOutput string, diffViaStdin bool) string {
	diffSection := diffRef
	if diffViaStdin {
		diffSection = "(diff provided via stdin)"
	}
	return fmt.Sprintf(`You are the adversarial reviewer in a two-agent coding workflow.
You cannot edit files. Do not propose broad refactors unless required
for correctness.

Original request:
%s

Diff under review (vs baseline %s):
%s

Test output:
%s

Evaluate: does the diff satisfy the request; correctness bugs; missed
edge cases; missing tests for changed behavior; unrelated modifications;
security/data-loss/migration risk; consistency with repo patterns.

Respond with JSON matching the provided schema. Use "pass" only when
there are no blocker or major findings. Every finding must name a file
and a concrete fix.`, userPrompt, baseline, diffSection, testOutput)
}

func BuildFixPrompt(round int, userPrompt, diff string, review Review) string {
	findingsJSON, _ := json.MarshalIndent(review, "", "  ")
	return fmt.Sprintf(`You are the coder in round %d. The adversarial reviewer found issues
with your previous change.

Original request:
%s

Reviewer findings (fix all blocker and major items):
%s

Current diff vs baseline:
%s

Fix the findings, keep the original request satisfied, avoid unrelated
changes, update tests as needed. Finish with: fixes made, checks run,
any finding you dispute and why.`, round, userPrompt, string(findingsJSON), diff)
}

func BuildSupervisorBriefPrompt(workdir, userPrompt string, canEdit bool) string {
	editNote := "You do not edit files yourself; the worker will implement the change."
	if canEdit {
		editNote = "You may make small exploratory edits if needed, but the worker does the primary implementation."
	}
	return fmt.Sprintf(`You are the supervisor in a supervisor-worker coding workflow. Before any
code is written, produce a compact implementation brief for the worker who
will make the change. %s

Repository: %s

Request:
%s

Write a compact implementation brief: the concrete approach, the files or
areas likely involved, edge cases to handle, and how to verify the change.
Do not write the diff yourself. Keep it short and actionable.`, editNote, workdir, userPrompt)
}

func BuildWorkerImplementPrompt(workdir, userPrompt, brief string) string {
	return fmt.Sprintf(`You are the worker in a supervisor-worker coding workflow. A supervisor
will review your diff; it does not edit files by default.

Complete this request in the repository at %s:

%s

Supervisor's implementation brief:
%s

Rules:
- Edit files directly. Do not describe a plan instead of implementing.
- Make the smallest correct change that satisfies the request.
- Follow the repository's existing style and architecture.
- Add or update tests when behavior changes.
- Leave unrelated files alone.

Finish with a concise summary: files changed, behavior changed,
checks run, known remaining risk.`, workdir, userPrompt, brief)
}

func BuildWorkerFixPrompt(round int, userPrompt, diff string, review Review) string {
	findingsJSON, _ := json.MarshalIndent(review, "", "  ")
	return fmt.Sprintf(`You are the worker in round %d. The supervisor found issues with your
previous change.

Original request:
%s

Supervisor findings (fix all blocker and major items):
%s

Current diff vs baseline:
%s

Fix the findings, keep the original request satisfied, avoid unrelated
changes, update tests as needed. Finish with: fixes made, checks run,
any finding you dispute and why.`, round, userPrompt, string(findingsJSON), diff)
}

func BuildSupervisorReviewPrompt(userPrompt, baseline, diffRef, testOutput string, diffViaStdin bool) string {
	diffSection := diffRef
	if diffViaStdin {
		diffSection = "(diff provided via stdin)"
	}
	return fmt.Sprintf(`You are the supervisor reviewing the worker's diff in a supervisor-worker
coding workflow. You cannot edit files. Do not propose broad refactors
unless required for correctness.

Original request:
%s

Diff under review (vs baseline %s):
%s

Test output:
%s

Evaluate: does the diff satisfy the request; correctness bugs; missed
edge cases; missing tests for changed behavior; unrelated modifications;
security/data-loss/migration risk; consistency with repo patterns.

Respond with JSON matching the provided schema. Use "pass" only when
there are no blocker or major findings. Every finding must name a file
and a concrete fix.`, userPrompt, baseline, diffSection, testOutput)
}

func BuildRoundLimitReportPrompt(roleLabel, counterpartLabel string, mode Mode, userPrompt, diff string, review Review, tests []TestRun) string {
	findingsJSON, _ := json.MarshalIndent(review, "", "  ")
	testsJSON, _ := json.MarshalIndent(tests, "", "  ")
	modeName := string(mode)
	if modeName == "" {
		modeName = string(ModeAdversarial)
	}
	return fmt.Sprintf(`You are the %s in a %s tagteam run.

The user-defined round limit has been reached. The run is done. Do not edit
files, do not request another round, and do not try to complete more work.
It is acceptable if the change is incomplete, if you disagree with the %s,
or if you would not call the result a pass; that disagreement is useful
signal for the human.

Original request:
%s

Latest review:
%s

Current diff vs baseline:
%s

Test history:
%s

Report, concisely:
- What remains incomplete or risky.
- Which findings you agree with, dispute, or could not verify.
- What you would do next if the human chose to continue.
- Whether you believe the current diff is acceptable as-is.`, roleLabel, modeName, counterpartLabel, userPrompt, string(findingsJSON), diff, string(testsJSON))
}
