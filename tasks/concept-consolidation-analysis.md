# Analýza: konsolidace konceptů (session typy, presety, workflows, schedules, knowledge)

*2026-07-29 — podklad k rozhodnutí, zatím nic neimplementováno.*

CodeForge má dnes pět způsobů, jak spustit „předpromptovanou" session. Rozjely se historicky (workflow→preset refactor, CI Action, včerejší presety) a překrývají se. Tenhle dokument je inventura + návrh cílového modelu + plán ve třech fázích.

## 1. Inventář

| Koncept | Co drží | Parametry | Builtin obsah | Jde naplánovat (cron)? |
|---|---|---|---|---|
| **Session type** (`internal/prompt`) | prompt šablonu + režim (read-only vs. edit) | jen volný prompt | 4: code, plan, review, pr_review | ne — Schedules typ neumí vybrat |
| **Preset** | uložený kompletní session request | prompt override při runu | žádný | ne |
| **Workflow definition** | šablonu s `{{.Params}}` + právě 1 session krok | ano, typované (required/default) | 1: sentry-fixer | ne |
| **Workflow config** | uložené hodnoty parametrů workflow | — | — | ne |
| **Schedule** | cron + surový session request | ne | — | ano (ale jen implicitní `code` s povinným promptem) |
| **Knowledge** (`internal/prompt/knowledge.go`) | analýza repa → `.codeforge/*.md` docs | `focus` | 2 kvalitní prompty | jen přes CI Action (`knowledge_update`) |

## 2. Hlavní nálezy

1. **Workflow = parametrizovaný preset.** Po refactoringu handler vynucuje „exactly one session step" (`internal/server/handlers/workflow.go:67`). Definice + config + run je totéž co preset + parametry. Tři koncepty (s configy) na jednu věc.
2. **Knowledge je schovaný klenot.** `AnalyzeRepoPrompt`/`UpdateKnowledgePrompt` jsou dobře napsané (update ne overwrite; stabilní znalosti, ne čísla řádků), ale ze serveru/UI nedostupné — existují jen jako `knowledge_update` v CI Action (`cmd/codeforge-action/ci_executor.go:216`). „Týdenní refresh projektové dokumentace" je přitom učebnicový use-case pro Schedules.
3. **Schedules neumí session type** — formulář posílá vždy `code` a prompt je povinný (`internal/server/handlers/schedules.go:45`), takže „každou neděli repo review" nejde naklikat, přestože backend `session_request.session_type` podporuje.
4. **Docs lžou o builtinech**: `docs/architecture.md:157` slibuje `github-issue-fixer`, `gitlab-issue-fixer`, `code-review`, `knowledge-update` — v `internal/workflow/builtins.go` existuje jen `sentry-fixer`. UI má pro neexistující builtin workflows připravené ikony (`WorkflowList.tsx`, `WorkflowCreate.tsx`).
5. **Session typy nejsou dokumentované** — žádná stránka v docs neříká, co Plan/Repo review dělají, že jsou read-only, a jaký JSON kontrakt review vrací.

## 3. Kvalita promptů

Vesměs dobrá: jasná read-only pravidla, review má vynucený JSON kontrakt, `pr_review` sladěný s backend validací diff řádků. Sentry-fixer builtin je nejpropracovanější (commit per fix, zákaz stub fixů, kontrakt na PR summary). Slabiny:

- `review.md` a `pr_review.md` **duplikují JSON schéma** a už driftují: `pr_review` má `auto_fixable`, `review` ne (parser čte obojí).
- `plan.md` **nemá strukturu výstupu** (sekce: soubory, přístup, rizika, složitost) — výstup je pokaždé jinak tvarovaný.
- `code` nemá šablonu žádnou (legitimní, ale nezdokumentované).

## 4. Cílový mentální model: CO × KDY

- **CO = Blueprint** (jeden koncept místo presety+workflow definice+workflow configy): pojmenovaná šablona session — repo, session type, prompt (klidně s `{{params}}`), MCP nástroje, auto-PR flagy. Preset = blueprint bez parametrů; workflow = blueprint s parametry; config = uložené hodnoty. Builtin knihovna: `sentry-fixer`, `knowledge-update`, `weekly-repo-review`.
- **KDY = Trigger**: ručně / cron (Schedule odkazuje na blueprint) / webhook (dnešní auto PR-review je fakticky builtin webhook trigger).
- **Session type zůstává** jako nízkoúrovňový „režim" (šablona + práva); blueprinty ho skládají, New session ho dál nabízí přímo.

## 5. Plán ve třech fázích

### Fáze 1 — rychlé srovnání (bez bourání, ~den)
- Session type výběr + volitelný prompt v Schedules formuláři (backend: zmírnit povinnost promptu pro review/plan typy).
- `knowledge` jako plnohodnotný serverový session type (prompty existují; read-only analýza + zápis do `.codeforge/`), tím pádem schedulovatelný.
- Sjednotit review JSON schéma do jednoho místa (jeden zdroj pro `review.md` i `pr_review.md`).
- Struktura výstupu pro `plan.md` (Files / Approach / Risks / Complexity).
- Nová `docs/session-types.md` + zmínka v README; opravit seznam builtinů v `architecture.md`.

### Fáze 2 — konsolidace CO
- Merge Workflows + Presets → blueprinty: migrace `workflow_configs` → presety s params, deprecated aliasy na `/workflows` API (pozor na CI/externí konzumenty), jedna UI stránka místo dvou.

### Fáze 3 — konsolidace KDY
- Schedule odkazuje na blueprint (`preset_id` + params override) místo surového JSON.
- „Schedule this" tlačítko na blueprintu i na hotové session.

## Doporučení

Fáze 1 = čistý zisk bez rizika, odemyká naplánované review a knowledge z UI. Fáze 2 je správná, ale bourá API — chce předem migrační návrh k odsouhlasení. Fáze 3 navazuje přirozeně na 2.
