# BE Refactor: Review & MR-Centric Workflow

## Cil

Presunout veskere review interakce do GitHub/GitLab MR. Task = session (ne jednorazova akce).
Uzivatel interaguje s CodeForge pres komentare v MR (`/review`, `/fix-cr`) nebo pres UI.

---

## 1. State Machine - `completed` a `pr_created` nejsou terminalni

### Soubor: `internal/task/state.go`

**Aktualni stav:**
```go
StatusCompleted:  {StatusAwaitingInstruction, StatusCreatingPR, StatusReviewing},
StatusPRCreated:  {StatusAwaitingInstruction, StatusCompleted},
```

**Pozadovana zmena:**
```go
StatusCompleted:  {StatusAwaitingInstruction, StatusCreatingPR, StatusReviewing},
StatusPRCreated:  {StatusAwaitingInstruction, StatusReviewing, StatusCreatingPR},
```

Zmena: `StatusPRCreated` musi umoznit prechod do `StatusReviewing` (review primo po vytvoreni MR) a `StatusCreatingPR` (update existujiciho PR po fixech).

**`IsFinished()` zmena:**
```go
func IsFinished(s TaskStatus) bool {
    return s == StatusFailed
    // completed a pr_created uz NEJSOU terminalni - task je session
}
```

> **Pozor:** Toto ma dopad na TTL v Redis. Aktualne se na terminal states nastavuje TTL.
> Reseni: TTL nastavovat jen na `StatusFailed`. Pro `completed`/`pr_created` pouzit
> delsi "idle TTL" (napr. 7 dni od posledni aktivity), ktery se resetuje pri kazde interakci.

---

## 2. Novy webhook handler pro MR komentare

### Soubor: `internal/server/handlers/webhook_receiver.go`

Aktualne handler zpracovava jen:
- GitHub: `pull_request` event (opened, synchronize, reopened)
- GitLab: `Merge Request Hook` (open, update, reopen)

**Pridat:**

### GitHub: `issue_comment` event
```go
// GitHubCommentWebhook handles POST /api/v1/webhooks/github
// pro event type "issue_comment" na PR
func (h *WebhookReceiverHandler) handleGitHubComment(w http.ResponseWriter, body []byte) {
    // 1. Parse issue_comment event
    // 2. Zkontrolovat ze comment je na PR (event.issue.pull_request != nil)
    // 3. Parsovat command z body: /review, /fix-cr, /fix <instrukce>
    // 4. Najit existujici task pro tento repo+PR (viz bod 3 nize)
    // 5. Podle commandu:
    //    - /review  -> StartReviewAsync() na task
    //    - /fix-cr  -> Instruct() s kontextem review komentaru
    //    - /fix <x> -> Instruct() s custom promptem
}
```

#### GitHub Comment Event struktura:
```go
type githubCommentEvent struct {
    Action  string `json:"action"` // "created"
    Comment struct {
        Body    string `json:"body"`
        User    struct {
            Login string `json:"login"`
        } `json:"user"`
    } `json:"comment"`
    Issue struct {
        Number      int `json:"number"`
        PullRequest *struct {
            URL string `json:"url"`
        } `json:"pull_request"` // non-nil = je to PR
    } `json:"issue"`
    Repository struct {
        FullName string `json:"full_name"`
        CloneURL string `json:"clone_url"`
    } `json:"repository"`
}
```

### GitLab: `Note Hook` event
```go
// Analogicky pro GitLab - "Note Hook" s noteable_type == "MergeRequest"
type gitlabNoteEvent struct {
    ObjectKind       string `json:"object_kind"` // "note"
    ObjectAttributes struct {
        Note         string `json:"note"`    // text komentare
        NoteableType string `json:"noteable_type"` // "MergeRequest"
        NoteableID   int    `json:"noteable_id"`
    } `json:"object_attributes"`
    MergeRequest struct {
        IID          int    `json:"iid"`
        SourceBranch string `json:"source_branch"`
        TargetBranch string `json:"target_branch"`
    } `json:"merge_request"`
    Project struct {
        PathWithNamespace string `json:"path_with_namespace"`
        HTTPURLToRepo     string `json:"http_url_to_repo"`
    } `json:"project"`
}
```

### Command parsing:
```go
// parseCommand extracts a forge command from a comment body.
// Returns command name and optional argument.
func parseCommand(body string) (cmd string, arg string) {
    // Hledame na zacatku radku nebo sam o sobe:
    // /review           -> ("review", "")
    // /fix-cr           -> ("fix-cr", "")
    // /fix refactor XY  -> ("fix", "refactor XY")
    // /codeforge review -> ("review", "")
}
```

### Router zmena v `internal/server/server.go`:
```go
// Aktualni:
r.Post("/api/v1/webhooks/github", webhookHandler.GitHubWebhook)
r.Post("/api/v1/webhooks/gitlab", webhookHandler.GitLabWebhook)

// Zmena: ve STEJNEM handleru rozlisit event type z headeru.
// GitHub: X-GitHub-Event = "pull_request" vs "issue_comment"
// GitLab: X-Gitlab-Event = "Merge Request Hook" vs "Note Hook"
// Neni treba novy endpoint - staci rozsirit existujici handler.
```

---

## 3. Task <-> MR mapovani (lookup)

### Problem
Kdyz prijde `/fix-cr` komentare z MR #42, potrebujeme najit task ktery ten MR vytvoril
nebo ktery nad nim pracuje.

### Soubor: `internal/task/service.go` (nova metoda)

```go
// FindByPR finds the most recent active task for a given repo + PR/MR number.
func (s *Service) FindByPR(ctx context.Context, repoURL string, prNumber int) (*Task, error) {
    // 1. Zkusit Redis - prohledat aktivni tasky (index)
    // 2. Fallback na SQLite: SELECT ... WHERE repo_url = ? AND pr_number = ?
    //    ORDER BY updated_at DESC LIMIT 1
}
```

### Moznosti implementace:

**A) Redis index (doporuceno):**
Pridat sekundarni index pri vytvoreni PR:
```
codeforge:pr:{repo_url_hash}:{pr_number} -> task_id
```
Nastavit pri `CreatePR()` a pri vytvoreni `pr_review` tasku z webhooku.
Vymazat pri TTL expiry.

**B) SQLite query:**
Uz existuje `pr_number` field v task modelu + SQLite persistence.
Staci pridat index a query metodu. Pomalejsi, ale jednodussi.

**Doporuceni:** Zacit s B (SQLite query), pridat Redis index az kdyz bude bottleneck.

---

## 4. Review -> Fix orchestrace

### Problem
Kdyz prijde `/fix-cr`, potrebujeme:
1. Najit task
2. Stahnout review komentare z MR
3. Sestavit prompt "Fix these review issues: ..."
4. Zavolat Instruct()
5. Po dokonceni automaticky spustit review
6. Postovat vysledek zpet do MR

### Soubor: novy `internal/task/orchestrator.go` nebo rozsireni `service.go`

```go
// FixFromReview orchestrates: fetch review comments -> instruct -> auto-review -> post
func (s *Service) FixFromReview(ctx context.Context, taskID string, opts FixFromReviewOptions) error {
    task, err := s.Get(ctx, taskID)
    if err != nil {
        return err
    }

    // 1. Fetch review comments z GitHub/GitLab
    comments, err := s.fetchReviewComments(ctx, task)

    // 2. Sestavit fix prompt
    prompt := buildFixPrompt(comments, opts.AdditionalInstruction)

    // 3. Instruct (pouzit existujici metodu)
    _, err = s.Instruct(ctx, taskID, prompt)

    // 4. Po dokonceni worker automaticky spusti review + post
    //    -> viz bod 5 nize (auto-review flag)
    return err
}
```

### Fetch review comments:

```go
// fetchReviewComments gets the latest review comments from the PR/MR.
// Soubor: internal/tool/git/comments.go (novy)

// GitHub: GET /repos/{owner}/{repo}/pulls/{pr}/reviews + /comments
func FetchGitHubReviewComments(ctx context.Context, repo RepoInfo, token string, prNumber int) ([]ReviewComment, error)

// GitLab: GET /projects/{id}/merge_requests/{mr}/discussions
func FetchGitLabReviewComments(ctx context.Context, repo RepoInfo, token string, mrIID int) ([]ReviewComment, error)

type ReviewComment struct {
    Author   string
    Body     string
    File     string // empty = general comment
    Line     int
    Path     string
    Created  time.Time
}
```

---

## 5. Auto-review po fix iteraci

### Problem
Po `/fix-cr` chceme automaticky spustit review a postovat vysledek.
Aktualne se tohle deje jen u `pr_review` tasku s `output_mode: "post_comments"`.

### Soubor: `internal/task/model.go`

Pridat do `TaskConfig`:
```go
type TaskConfig struct {
    // ... existujici fields ...

    // AutoReviewAfterFix - po kazde iteraci automaticky spustit review + post
    AutoReviewAfterFix bool `json:"auto_review_after_fix,omitempty"`

    // AutoPostReview - automaticky postovat review vysledek do MR
    AutoPostReview bool `json:"auto_post_review,omitempty"`
}
```

### Soubor: `internal/worker/executor.go`

V `Execute()` po uspesnem dokonceni tasku (Phase 4 finalize):
```go
// Po dokonceni iterace - zkontrolovat auto-review
if task.Config != nil && task.Config.AutoReviewAfterFix && task.Iteration > 1 {
    // Automaticky spustit review
    if err := e.taskService.StartReviewAsync(ctx, task.ID, task.Config.CLI, task.Config.AIModel); err != nil {
        slog.Error("auto-review failed to start", "task_id", task.ID, "error", err)
    }
}
```

V `executeReview()` po uspesnem dokonceni review:
```go
// Po review - automaticky postovat do MR pokud je nastaveno
if task.Config != nil && task.Config.AutoPostReview && task.PRNumber > 0 {
    // Postovat review komentare
    // (analogie handlePRReviewCompletion z executor.go)
}
```

---

## 6. PR/MR creation - Sentry link v description

### Soubor: `internal/tool/git/github.go` a `gitlab.go`

`PRCreateOptions` uz ma `Body string` field. Staci ho naplnit.

### Soubor: kde se vola `CreatePR`

Task handler `CreatePR` v `handlers/tasks.go`:
```go
// Aktualne:
opts := git.PRCreateOptions{
    Title: req.Title,
    Body:  req.Body,  // <- uz existuje, jen se nepouziva dobre
    ...
}

// Zmena: pokud task ma metadata (sentry_issue_url, sentry_issue_id),
// vlozit do body:
// "Fixes: [SENTRY-123](https://sentry.io/...)"
```

Alternativne: toto muze resit klient (UI/CI) ktery posle body s linkem.
Ale lepsi je to resit na BE, protoze BE ma pristup k task metadata.

### Soubor: `internal/task/model.go`

Pridat do Task:
```go
type Task struct {
    // ... existujici fields ...

    // Metadata - volitelna key-value data (sentry URL, ticket link, etc.)
    Metadata map[string]string `json:"metadata,omitempty"`
}
```

Workflow sentry-fixer uz muze metadata nastavit pri vytvoreni tasku.

---

## 7. Update existujiciho PR po fixech

### Problem
Kdyz task udela fix po review, potrebujeme pushnout zmeny do EXISTUJICIHO PR branche,
ne vytvaret novy PR.

### Aktualni stav
`CreatePR()` v `github.go`/`gitlab.go` vytvori NOVY PR.
Task ma field `PRURL` a `PRNumber` po vytvoreni.

### Reseni
Pricaz push uz existuje (pouziva se pri CreatePR). Staci:

1. Pokud task.PRNumber > 0 a task.Branch existuje:
   - Push zmeny do existujici branch (ne vytvaret novy PR)
   - Volitelne update PR description

2. Novy endpoint nebo rozsireni existujiciho:
```go
// POST /api/v1/tasks/{id}/push
// Pushne aktualni zmeny do branch bez vytvareni noveho PR.
// Pokud PR uz existuje (task.PRNumber > 0), jen push.
// Pokud PR neexistuje, vrati chybu "use create-pr first".
```

Alternativne: rozsirit `POST /api/v1/tasks/{id}/create-pr` aby detekoval
existujici PR a jen pushnul misto vytvareni noveho.

---

## 8. CI Action - podpora comment commands

### Soubor: `cmd/codeforge-action/main.go`

Pridat novy task type:
```go
validTypes := map[string]bool{
    "pr_review":        true,
    "code_review":      true,
    "knowledge_update": true,
    "custom":           true,
    "fix_review":       true,  // NOVY: fix na zaklade review komentaru
}
```

### Soubor: `cmd/codeforge-action/ci_executor.go`

```go
case "fix_review":
    return e.handleFixReview(ctx, ciCtx)
```

`handleFixReview`:
1. Fetch review komentare z PR (pouzit existujici git package)
2. Sestavit prompt: "Fix these review issues: ..."
3. Spustit CLI
4. Commitnout a pushnout zmeny
5. Volitelne spustit follow-up review

---

## 9. Webhook routing zmena

### Soubor: `internal/server/handlers/webhook_receiver.go`

Aktualni `GitHubWebhook` handler rozlisit podle event type:

```go
func (h *WebhookReceiverHandler) GitHubWebhook(w http.ResponseWriter, r *http.Request) {
    // ... signature verification (spolecna) ...

    eventType := r.Header.Get("X-GitHub-Event")
    switch eventType {
    case "pull_request":
        h.handleGitHubPR(w, r, body)
    case "issue_comment":
        h.handleGitHubComment(w, r, body)
    default:
        writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
    }
}
```

Analogicky pro GitLab:
```go
func (h *WebhookReceiverHandler) GitLabWebhook(w http.ResponseWriter, r *http.Request) {
    // ... token verification ...

    eventType := r.Header.Get("X-Gitlab-Event")
    switch eventType {
    case "Merge Request Hook":
        h.handleGitLabMR(w, r, body)
    case "Note Hook":
        h.handleGitLabNote(w, r, body)
    default:
        writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
    }
}
```

---

## 10. Webhook konfigurace - registrace eventu

### GitHub webhook setup
Pri registraci webhooku pridat eventy:
- `pull_request` (uz je)
- `issue_comment` (NOVY - pro PR komentare)

### GitLab webhook setup
Pri registraci webhooku pridat trigger:
- `Merge request events` (uz je)
- `Note events` (NOVY - pro MR komentare)

> Toto je konfigurace na strane GitHubu/GitLabu, ne zmena v kodu.
> Ale docs/README by mely popisovat jake eventy registrovat.

---

## Souhrn zmen po souborech

| Soubor | Zmena | Slozitost |
|--------|-------|-----------|
| `internal/task/state.go` | Upravit transitions, IsFinished | Mala |
| `internal/task/model.go` | Pridat Metadata, TaskConfig fields | Mala |
| `internal/task/service.go` | Pridat FindByPR, FixFromReview, upravit TTL logiku | Stredni |
| `internal/server/handlers/webhook_receiver.go` | Pridat comment/note handling, command parsing | Stredni |
| `internal/tool/git/comments.go` | NOVY - fetch review comments z GitHub/GitLab | Stredni |
| `internal/tool/git/github.go` | Pridat push-only (bez vytvoreni PR) | Mala |
| `internal/tool/git/gitlab.go` | Pridat push-only (bez vytvoreni MR) | Mala |
| `internal/worker/executor.go` | Auto-review po fix iteraci, auto-post | Stredni |
| `internal/server/handlers/tasks.go` | Pridat push endpoint, metadata v create-pr | Mala |
| `cmd/codeforge-action/main.go` | Pridat fix_review task type | Mala |
| `cmd/codeforge-action/ci_executor.go` | Pridat handleFixReview | Stredni |

---

## Poradi implementace (doporucene)

### Faze 1: Zaklad (umozni loop)
1. `state.go` - upravit transitions + IsFinished
2. `model.go` - pridat Metadata, AutoReviewAfterFix, AutoPostReview
3. `service.go` - upravit TTL logiku pro non-terminal completed/pr_created

### Faze 2: MR Comment Commands
4. `webhook_receiver.go` - pridat GitHub issue_comment + GitLab Note handling
5. `webhook_receiver.go` - command parsing (/review, /fix-cr, /fix)
6. `service.go` - FindByPR metoda

### Faze 3: Review -> Fix orchestrace
7. `tool/git/comments.go` - fetch review comments
8. `service.go` - FixFromReview orchestrace
9. `executor.go` - auto-review po fix + auto-post

### Faze 4: Push & PR updates
10. `github.go` / `gitlab.go` - push bez vytvoreni noveho PR
11. `handlers/tasks.go` - push endpoint, metadata v PR body

### Faze 5: CI Action
12. `codeforge-action` - fix_review task type
