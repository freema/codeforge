# Fáze 2: Osekat builtins jen na sentry-fixer

**Commit:** `refactor: remove non-sentry builtin workflows`

## Co se maže

Z `internal/workflow/builtins.go` smazat workflow definice:
- `github-issue-fixer` (řádky 136-184)
- `gitlab-issue-fixer` (řádky 186-231)
- `knowledge-update` (řádky 233-278)

## Co zůstane

- `sentry-fixer` definice (řádky 72-135)
- `SeedBuiltins()` funkce (řádky 284-324)
- `mustJSON()` helper (řádky 326-332)
- Prompty jsou už přesunuté do `internal/prompt/` (fáze 1), takže import `prompt` balíčku se může z builtins.go odstranit

## Vedlejší efekt

`SeedBuiltins()` při startu automaticky smaže stale builtiny z DB (řádky 291-303). Takže po deployi se github-issue-fixer, gitlab-issue-fixer a knowledge-update samy odstraní z `workflows` tabulky.

## Soubory

| Soubor | Akce |
|--------|------|
| `internal/workflow/builtins.go` | Smazat 3 workflow definice, odstranit prompt import |
| `internal/workflow/builtins_test.go` | Upravit test count (4 → 1 workflow) |

## Ověření

```bash
task build
task test
```
