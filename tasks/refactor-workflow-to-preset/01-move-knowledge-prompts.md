# Fáze 1: Přesunout knowledge-update prompty

**Commit:** `refactor: move knowledge prompts to prompt package`

## Proč

CI Action (`cmd/codeforge-action/ci_executor.go:252`) importuje `workflow.AnalyzeRepoPrompt` a `workflow.UpdateKnowledgePrompt`. Než začneme řezat `builtins.go`, musíme prompty přesunout jinam, aby CI Action neshořel.

## Kroky

### 1. Vytvořit prompty v `internal/prompt/`

Vytvořit soubor `internal/prompt/knowledge.go`:
- Přesunout `AnalyzeRepoPrompt` a `UpdateKnowledgePrompt` konstanty
- Žádné závislosti na workflow balíčku

### 2. Přepojit CI Action

Soubor: `cmd/codeforge-action/ci_executor.go`
- Změnit import z `workflow.AnalyzeRepoPrompt` → `prompt.AnalyzeRepoPrompt`
- Změnit import z `workflow.UpdateKnowledgePrompt` → `prompt.UpdateKnowledgePrompt`
- Řádek 252: `return prompt.AnalyzeRepoPrompt + "\n\n---\n\n" + prompt.UpdateKnowledgePrompt, nil`

### 3. Přepojit builtins.go (dočasně)

Soubor: `internal/workflow/builtins.go`
- Knowledge-update workflow definice odkazuje na `AnalyzeRepoPrompt` a `UpdateKnowledgePrompt`
- Přepojit na `prompt.AnalyzeRepoPrompt` a `prompt.UpdateKnowledgePrompt`
- (V další fázi se celý knowledge-update workflow smaže)

### Ověření

```bash
task build          # oba binárky se musí zkompilovat
task test           # testy musí projít
```
