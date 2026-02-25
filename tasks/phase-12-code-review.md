# Phase 12: Code Review

## Cíl

Automatický **code review** změn jiným AI modelem/agentem **před vytvořením PR**. Review agent zkontroluje kvalitu, bezpečnost a správnost kódu.

## Závislosti

- **Phase 9: Multi-CLI** — potřebujeme jiný CLI/model pro review
- **Phase 10: Multi-Agent** — review je speciální případ 2-agent pipeline

## Motivace

Aktuálně CodeForge vytvoří PR s kódem "as-is" od jednoho agenta. Code review:
- Zachytí bugy a security issues před PR
- Zlepší kvalitu kódu (naming, patterns, edge cases)
- Poskytne "second opinion" od jiného modelu
- Může automaticky opravit nalezené problémy

## Architektura

Code review může fungovat na dvou úrovních:

### Level 1: Post-task Review (jednodušší)
Review se spustí automaticky po dokončení tasku, **před** PR creation:

```
Task completed → Auto Review → Review passed? → Create PR
                                     ↓ NO
                            Fix suggestions → Re-run task
```

### Level 2: Pipeline Review (Phase 10)
Review jako agent v pipeline (viz Phase 10):

```json
{
  "pipeline": "code-and-review",
  "agents": [
    {"name": "coder", "cli": "claude-code"},
    {"name": "reviewer", "cli": "codex", "role": "reviewer"}
  ]
}
```

## Level 1: Post-task Auto Review

### Flow

```
┌──────────────────────────────────────────────────────────────┐
│  TASK COMPLETED                                               │
│  ├─ Changes: 5 files modified, +200 -50                      │
│  └─ Result: "Implemented authentication..."                   │
└──────────────────────┬───────────────────────────────────────┘
                       │
                       ▼ (auto_review: true)
┌──────────────────────────────────────────────────────────────┐
│  REVIEW AGENT                                                 │
│  ├─ CLI: {review_cli} (default: same as task, different model)│
│  ├─ Input: git diff + task prompt + result summary            │
│  ├─ Prompt: structured review prompt (see below)              │
│  └─ Output: ReviewResult                                      │
└──────────────────────┬───────────────────────────────────────┘
                       │
                       ▼
┌──────────────────────────────────────────────────────────────┐
│  REVIEW RESULT                                                │
│  ├─ verdict: "approve" | "request_changes" | "comment"        │
│  ├─ score: 8/10                                               │
│  ├─ issues: [{severity, file, line, description, suggestion}] │
│  ├─ summary: "Code is clean, minor naming issues..."          │
│  └─ auto_fixable: true/false                                  │
└──────────────────────┬───────────────────────────────────────┘
                       │
              ┌────────┴────────┐
              │                 │
              ▼                 ▼
        APPROVED          REQUEST_CHANGES
              │                 │
              ▼                 ▼
        Ready for PR      Auto-fix? ──→ Fix agent
                                         (iteration)
```

### Review Prompt Template

```
You are a senior code reviewer. Review the following changes carefully.

## Task Description
{task.Prompt}

## Changes Summary
{changes.FilesModified} files modified, {changes.DiffStats}

## Diff
{git diff output}

## Review Criteria
1. **Correctness**: Does the code correctly implement the task?
2. **Security**: Any security vulnerabilities (injection, XSS, auth bypass)?
3. **Performance**: Any obvious performance issues?
4. **Error Handling**: Are errors properly handled?
5. **Code Quality**: Naming, structure, readability?
6. **Tests**: Are changes covered by tests?

## Output Format
Respond with a JSON object:
{
  "verdict": "approve" | "request_changes" | "comment",
  "score": 1-10,
  "summary": "Brief overall assessment",
  "issues": [
    {
      "severity": "critical" | "major" | "minor" | "suggestion",
      "file": "path/to/file.go",
      "line": 42,
      "description": "What's wrong",
      "suggestion": "How to fix it"
    }
  ],
  "auto_fixable": true/false
}
```

## Datový model

```go
// internal/review/model.go

type ReviewConfig struct {
    Enabled     bool   `json:"enabled"`
    CLI         string `json:"cli,omitempty"`        // override CLI for review
    Model       string `json:"model,omitempty"`      // override model for review
    AutoFix     bool   `json:"auto_fix"`             // auto-fix minor issues
    MinScore    int    `json:"min_score"`             // minimum score to pass (default: 7)
    MaxRetries  int    `json:"max_retries"`           // max fix iterations (default: 2)
}

type ReviewResult struct {
    Verdict     string        `json:"verdict"`      // approve, request_changes, comment
    Score       int           `json:"score"`         // 1-10
    Summary     string        `json:"summary"`
    Issues      []ReviewIssue `json:"issues"`
    AutoFixable bool          `json:"auto_fixable"`
    ReviewedBy  string        `json:"reviewed_by"`   // CLI + model
    Duration    float64       `json:"duration_seconds"`
}

type ReviewIssue struct {
    Severity    string `json:"severity"`    // critical, major, minor, suggestion
    File        string `json:"file"`
    Line        int    `json:"line,omitempty"`
    Description string `json:"description"`
    Suggestion  string `json:"suggestion,omitempty"`
}

type ReviewVerdict string
const (
    VerdictApprove        ReviewVerdict = "approve"
    VerdictRequestChanges ReviewVerdict = "request_changes"
    VerdictComment        ReviewVerdict = "comment"
)
```

### Task Extension

```go
// Rozšíření CreateTaskRequest
type CreateTaskRequest struct {
    // ... existující ...
    Review *ReviewConfig `json:"review,omitempty"`
}

// Rozšíření Task
type Task struct {
    // ... existující ...
    ReviewResult *ReviewResult `json:"review_result,omitempty"`
}
```

### Task Request

```json
POST /api/v1/tasks
{
  "repo_url": "...",
  "prompt": "Add input validation to user registration",
  "review": {
    "enabled": true,
    "auto_fix": true,
    "min_score": 7,
    "model": "claude-sonnet-4-20250514"
  }
}
```

## Redis klíče

| Klíč | Typ | Popis |
|------|-----|-------|
| `task:{id}:review` | String | JSON-encoded ReviewResult |
| `task:{id}:review:diff` | String | Diff sent to reviewer |

## State Machine Extension

```
running → reviewing → completed (if approved)
                    → running (if auto-fix, back to coding)
                    → failed (if review failed, max retries)
```

Nové stavy:
- `reviewing` — review agent běží
- Transition: `running → reviewing` (po dokončení coding)
- Transition: `reviewing → completed` (review passed)
- Transition: `reviewing → running` (auto-fix iteration)

## Nové soubory

```
internal/review/
  model.go       — ReviewConfig, ReviewResult, ReviewIssue
  reviewer.go    — Review execution logic
  prompt.go      — Review prompt templates
  parser.go      — Parsování review output (JSON extraction)
```

## Tasky

### 12.1 — Datový model
- [ ] Vytvořit `internal/review/model.go`
- [ ] Rozšířit `CreateTaskRequest` o `Review` config
- [ ] Rozšířit `Task` model o `ReviewResult`
- [ ] Nový task status: `reviewing`
- [ ] Aktualizovat state machine

### 12.2 — Review Prompt
- [ ] Vytvořit `internal/review/prompt.go`
- [ ] Template pro review prompt
- [ ] Formátování diff contextu
- [ ] Truncation pro velké diffy

### 12.3 — Review Parser
- [ ] Vytvořit `internal/review/parser.go`
- [ ] Extrakce JSON z review output (agent může obalit textem)
- [ ] Validace ReviewResult struktury
- [ ] Fallback: pokud JSON parsing selže → default "comment" verdict
- [ ] Unit testy

### 12.4 — Reviewer
- [ ] Vytvořit `internal/review/reviewer.go`
- [ ] Spustí CLI s review promptem
- [ ] Parsuje výsledek
- [ ] Rozhodne: approve → pokračuj, request_changes → iterace/fail
- [ ] Stream events: `review_started`, `review_completed`, `review_issue_found`
- [ ] Unit testy

### 12.5 — Executor integrace
- [ ] Upravit `Executor.Execute()`:
  - Po úspěšném run: zkontroluj review config
  - Pokud review enabled: spusť review
  - Pokud approved: pokračuj k completed
  - Pokud request_changes + auto_fix: nová iterace
  - Pokud request_changes + no auto_fix: completed s review result
- [ ] Backward compatible

### 12.6 — Auto-fix Loop
- [ ] Implementovat fix iteration:
  - Review issues → prompt pro fix agenta
  - Max retries limit
  - Trackování fix attempts
- [ ] Prevent infinite loops

### 12.7 — PR Integration
- [ ] Review result do PR description (pokud PR se vytváří po review)
- [ ] Badge/label na PR: "AI Reviewed, Score: 8/10"
- [ ] Review issues jako PR comments (inline)

### 12.8 — Integration testy
- [ ] Test: task s review → approve → completed
- [ ] Test: task s review → request_changes → auto-fix → approve
- [ ] Test: task s review → max retries → fail
- [ ] Test: task bez review → beze změn

## Konfigurace

```yaml
review:
  default_enabled: false          # globální default
  default_cli: claude-code        # CLI pro review
  default_model: claude-sonnet-4-20250514  # model pro review
  default_min_score: 7
  default_max_retries: 2
  max_diff_chars: 50000           # max diff size pro review
```

## Testovací strategie

### Review Parser testy (`internal/review/parser_test.go`)

Kritická komponenta — parsuje nestrukturovaný AI output na strukturovaný ReviewResult:

- [ ] `TestParser_CleanJSON` — čistý JSON → ReviewResult
- [ ] `TestParser_JSONInMarkdown` — JSON obalený markdownem (```json...```)
- [ ] `TestParser_JSONWithPreamble` — text + JSON + text → extrahuj JSON
- [ ] `TestParser_InvalidJSON` — žádný JSON → fallback verdict "comment"
- [ ] `TestParser_PartialResult` — JSON bez všech polí → defaults
- [ ] `TestParser_ScoreOutOfRange` — score > 10 → clamp to 10
- [ ] `TestParser_ScoreOutOfRange_Low` — score < 1 → clamp to 1
- [ ] `TestParser_EmptyIssues` — verdict "approve" + prázdné issues → OK
- [ ] `TestParser_UnknownVerdict` — neznámý verdict → "comment"
- [ ] `TestParser_UnknownSeverity` — neznámá severity → "suggestion"

```go
func TestParser_JSONInMarkdown(t *testing.T) {
    input := "Here is my review:\n```json\n{\"verdict\":\"approve\",\"score\":9}\n```\nThat's all."
    result, err := ParseReviewOutput(input)
    if err != nil { t.Fatalf("Parse: %v", err) }
    if result.Verdict != "approve" { t.Errorf("Verdict = %q, want %q", result.Verdict, "approve") }
    if result.Score != 9 { t.Errorf("Score = %d, want %d", result.Score, 9) }
}
```

### Review Prompt testy (`internal/review/prompt_test.go`)

- [ ] `TestPrompt_Build` — generuje prompt se všemi sekcemi
- [ ] `TestPrompt_DiffTruncation` — diff > max_diff_chars se ořeže
- [ ] `TestPrompt_ContainsOutputFormat` — prompt obsahuje JSON format specifikaci
- [ ] `TestPrompt_ContainsCriteria` — prompt obsahuje všech 6 kritérií

### Reviewer testy (`internal/review/reviewer_test.go`)

S mock CLI runner (nikdy nevoláme reálné CLI v unit testech):

```go
type mockReviewRunner struct {
    output string
    err    error
}
func (m *mockReviewRunner) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
    return &RunResult{Output: m.output, ExitCode: 0}, m.err
}
```

- [ ] `TestReviewer_Approve` — mock vrátí approve → ReviewResult.Verdict == "approve"
- [ ] `TestReviewer_RequestChanges` — mock vrátí issues → ReviewResult s issues
- [ ] `TestReviewer_CLIFailure` — CLI error → fallback review result
- [ ] `TestReviewer_Timeout` — context deadline → error s timeout info

### State Machine testy (`internal/task/state_test.go` — rozšíření)

- [ ] `TestTransition_RunningToReviewing` — validní
- [ ] `TestTransition_ReviewingToCompleted` — validní (approved)
- [ ] `TestTransition_ReviewingToRunning` — validní (auto-fix)
- [ ] `TestTransition_ReviewingToFailed` — validní (max retries)
- [ ] `TestTransition_PendingToReviewing` — nevalidní

### Auto-fix Loop testy

- [ ] `TestAutoFix_SingleIteration` — review → fix → approve → done
- [ ] `TestAutoFix_MaxRetries` — 3 review iterations → max retries → fail
- [ ] `TestAutoFix_NoLoop` — auto_fix=false → no fix attempt

### Integration testy (`//go:build integration`)

- [ ] `TestIntegration_Review_Approve` — mock CLI approve → task completed
- [ ] `TestIntegration_Review_Disabled` — review disabled → standard flow
- [ ] `TestIntegration_Review_InPR` — review result v PR description

## Linter checklist

- [ ] JSON extraction regex je bezpečný (no catastrophic backtracking)
- [ ] Score clamping: vždy validní 1-10 rozsah
- [ ] Review timeout oddělený od task timeout
- [ ] `task fmt` + `task lint` MUSÍ projít

## Otevřené otázky

1. **Review cost** — review přidává latenci a náklady. Opt-in vs opt-out?
2. **Self-review** — může stejný model reviewovat svůj kód? (pravděpodobně ne ideální)
3. **Review persistence** — ukládat review výsledky pro analytics?
4. **Manual review** — umožnit "human-in-the-loop" review před PR?
5. **Review templates** — různé review criteria pro různé typy tasků?
