# 06 — AI Helper Layer (PR metadata, commit messages, summaries)

## Problém

Dnes `Analyzer` generuje PR metadata hloupě — truncuje prompt a přidá "CodeForge: " prefix. Výsledek: `CodeForge: Uprav mi readme prosím moc děkuji`. Commit messages jsou generické ("follow-up changes"). Přitom máme AI API klíče v registru.

## Cíl

Nový `internal/ai/` package — lehký HTTP klient na Anthropic/OpenAI API pro generování krátkých textů. Používá se jako helper pro:
- PR title a description (z diffu)
- Commit messages (z diffu)
- Branch name slug
- Budoucnost: session summary, release notes, changelog

## Návrh

### Architektura

```
internal/ai/
  client.go       — HTTP klient (Anthropic + OpenAI), interface
  prompts/        — .md šablony pro AI helper calls (embed FS)
    pr_metadata.md
    commit_message.md
```

### Client interface

```go
type Client interface {
    Generate(ctx context.Context, system, user string) (string, error)
}
```

Dvě implementace:
- `AnthropicClient` — POST `https://api.anthropic.com/v1/messages` s `x-api-key`
- `OpenAIClient` — POST `https://api.openai.com/v1/chat/completions` s `Bearer`

### Fallback chain

```
1. Anthropic API key v registru? → AnthropicClient (model: claude-haiku — rychlý, levný)
2. OpenAI API key v registru? → OpenAIClient (model: gpt-4.1-mini — rychlý, levný)
3. Žádný klíč? → fallback na stávající hloupý Analyzer (truncate prompt)
```

Inicializace v `main.go`:
```go
aiClient := ai.NewClientFromKeys(keyRegistry)  // returns Client or nil
analyzer := runner.NewAnalyzer(aiClient)        // accepts nil = fallback mode
```

### Prompt šablony

**`prompts/pr_metadata.md`**:
```
Generate a PR title and description for the following changes.

## Diff
{{.Diff}}

## Original task
{{.Prompt}}

Respond in JSON:
{"title": "short conventional commit style title", "description": "markdown description"}

Rules:
- Title max 72 chars, conventional commit format (feat:, fix:, docs:, refactor:)
- Description: 2-3 bullet points summarizing changes
- Language: match the language of the original task prompt
```

**`prompts/commit_message.md`**:
```
Generate a git commit message for these changes.

## Diff
{{.Diff}}

Respond with just the commit message, no quotes. Max 72 chars first line.
Use conventional commit format (feat:, fix:, docs:, refactor:).
```

### Integrace

#### Analyzer (PR metadata)
```go
func (a *Analyzer) Analyze(ctx context.Context, prompt, taskID string) *AnalysisResult {
    if a.ai != nil {
        // Try AI generation
        diff := getDiffFromWorkspace(taskID)
        result, err := a.generateWithAI(ctx, prompt, diff)
        if err == nil {
            return result
        }
        slog.Warn("AI metadata generation failed, using fallback", "error", err)
    }
    // Fallback: truncate prompt (stávající chování)
    return a.fallbackAnalyze(prompt, taskID)
}
```

#### Commit message (PushToPR)
```go
commitMsg := "follow-up changes"  // default
if s.ai != nil {
    if generated, err := s.ai.GenerateCommitMessage(ctx, diff); err == nil {
        commitMsg = generated
    }
}
```

### Prompt soubory — pravidlo

**Všechny prompty MUSÍ být v souborech, nikdy v kódu:**
- Session type prompty: `internal/prompt/templates/*.md` (existující)
- AI helper prompty: `internal/ai/prompts/*.md` (nové)
- Workflow prompty: přesunout z `builtins.go` do `internal/prompt/templates/` (budoucí refactor)

### Konfigurace

Žádná extra konfigurace — klíče se berou z existujícího key registru. Model se vybere automaticky (nejlevnější/nejrychlejší).

Volitelně v budoucnu:
```yaml
ai:
  helper_model: "claude-haiku-4-5"  # override
  enabled: true                      # kill switch
```

### API limity a bezpečnost

- Timeout: 10s per call (helper calls musí být rychlé)
- Max diff size: 4000 chars (truncate, ne celý diff)
- Retry: 1x s 2s delay
- Nikdy neposlat citlivá data (tokeny, secrets) do AI
- Rate limit: max 10 helper calls/min (guard proti runaway loops)

## Dotčené soubory

- `internal/ai/` — nový package
- `internal/ai/prompts/` — šablony
- `internal/tool/runner/analyzer.go` — rozšířit o AI fallback
- `internal/session/pr_service.go` — AI commit messages
- `cmd/codeforge/main.go` — inicializace AI clienta
- `internal/workflow/builtins.go` — budoucí: přesunout prompty do souborů

## Fáze

1. **Fáze 1**: `ai.Client` + PR title/description generování
2. **Fáze 2**: Commit message generování
3. **Fáze 3**: Přesun workflow promptů do souborů
4. **Fáze 4**: Session summary, release notes

## Otevřené otázky

- Chceme cache na AI responses? (stejný diff → stejný title)
- Má uživatel mít možnost editovat AI-generovaný title před vytvořením PR?
- Jaký model pro helper calls? (haiku je nejlevnější ale sonnet je chytřejší)
