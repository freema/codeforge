# 03 — Cross-Review Pattern

## Problém

Dnes CodeForge podporuje review jako jednosměrnou akci — AI reviewuje kód, ale nikdo neověřuje kvalitu reviewu ani neřeší situaci, kdy dva agenti pracují na různých částech a potřebují si vzájemně zkontrolovat kód.

## Cíl

Zavést **cross-review pattern** — mechanismus, kde jedna AI session reviewuje výstup jiné AI session v rámci stejného workflow, s možností iterativních oprav.

## Návrh

### Dva přístupy

#### A) Review jako krok v pipeline (doporučeno)

Nejjednodušší varianta — review je normální `agent_session` krok s rolí `reviewer`:

```yaml
steps:
  - name: implement
    role: implementer
    prompt: "Implement feature X"

  - name: review
    role: reviewer
    workspace_ref: implement
    depends_on: [implement]
    session_type: review     # → výstup je strukturovaný ReviewResult
    prompt: "Review the changes made by the implementer"

  - name: fix
    role: implementer
    workspace_ref: implement
    depends_on: [review]
    condition: "{{if .Steps.review.review_result.verdict | eq 'request_changes'}}true{{end}}"
    prompt: "Fix review issues: {{.Steps.review.result}}"
```

Výhody: využívá existující review systém, žádná nová infrastruktura.

#### B) Review loop (iterativní)

Pro produkční kvalitu — opakovat review → fix cyklus dokud reviewer neschválí:

```yaml
steps:
  - name: implement
    role: implementer

  - name: review_loop
    type: review_loop           # nový step typ
    reviewer_role: reviewer
    fixer_role: implementer
    workspace_ref: implement
    max_iterations: 3           # guard proti nekonečné smyčce
    exit_on: approve            # ukončit když reviewer schválí
```

Interně review_loop vytváří dynamické sub-steps dokud nedostane `approve` verdict nebo nevyčerpá `max_iterations`.

### Dvou-vývojářový model

Pro scénáře s dvěma paralelními implementátory (budoucnost):

```yaml
steps:
  - name: dev1_implement
    role: implementer
    prompt: "Implement stories 1, 3, 5"

  - name: dev2_implement
    role: implementer
    prompt: "Implement stories 2, 4, 6"

  - name: dev1_reviews_dev2
    role: reviewer
    workspace_ref: dev2_implement
    depends_on: [dev2_implement]

  - name: dev2_reviews_dev1
    role: reviewer
    workspace_ref: dev1_implement
    depends_on: [dev1_implement]
```

### Integrace s existujícím review systémem

- `session_type: review` v agent_session kroku → executor použije review template a parsuje ReviewResult
- ReviewResult z review kroku dostupný v `{{.Steps.review.review_result}}` pro podmíněné kroky
- Pokud je review v rámci pipeline, výsledek se **nepostuje na PR** (interní review), ale ukládá se do workflow run

### Metriky

Nové metriky pro cross-review:
- `codeforge_review_loops_total` — počet review iterací v pipeline
- `codeforge_review_verdicts_total{verdict}` — distribuce verdiktů (approve/request_changes/comment)
- `codeforge_review_auto_fixed_total` — počet úspěšně opravených review issues

## Dotčené soubory

- `internal/workflow/model.go` — podmíněné kroky, (volitelně) review_loop step typ
- `internal/workflow/orchestrator.go` — evaluace podmínek, (volitelně) loop logika
- `internal/workflow/step_session.go` — předání review_result do kontextu dalších kroků
- `internal/metrics/` — nové metriky

## Fáze implementace

1. **Fáze 1**: Review jako pipeline step + podmíněný fix krok (varianta A)
2. **Fáze 2**: Review loop step typ (varianta B)
3. **Fáze 3**: Dvou-vývojářový model s paralelními kroky

## Otevřené otázky

- Má reviewer vidět diff nebo celý workspace?
- Jak zabránit "echo chamber" efektu kde review vždy schválí?
- Chceme logy z review loop streamovat jako sub-events workflow streamu?
