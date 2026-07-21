# CodeForge UI — "The Forge" design system

Identity: an industrial control room for a forge. Warm iron charcoal (dark) /
workshop paper (light), ivory text, one molten-orange ember accent. Quiet,
matte, precise. **The only thing that glows is what is actually running.**

## Two hard rules

1. **Machine data is mono, human UI is sans.** Session IDs, repo names,
   branches, versions, cron expressions, timestamps, log/stream output, key
   names → `font-mono` (IBM Plex Mono). Labels, headings, buttons, body copy →
   default sans (Archivo). Never mix within one element.
2. **One glow.** `animate-ember` (breathing ember glow) marks live activity
   only: running/cloning/reviewing indicators, live stream dot. Nothing else
   glows — no text-shadows, no colored box-shadows, no neon.

## Tokens (all defined in `src/index.css`, use via Tailwind utilities)

- Surfaces: `bg-page` (app), `bg-surface` (card), `bg-surface-alt` (raised/
  nested), `bg-input` (form fields), `border-edge` (all hairlines)
- Text: `text-fg` (primary), `text-fg-2` (secondary), `text-fg-3` (muted),
  `text-fg-4` (faint)
- Accent (ember): `accent`, `accent-bold` (pressed), `accent-hover`,
  `accent-soft` (tinted bg), `accent-muted` (tinted border)
- Status: `ok` (tempered green — success/completed), `danger` (rust — failed/
  destructive), `warn` (amber — queued/waiting), `info` (steel — cold process:
  cloning, reviewing, PR). Tint with opacity: `bg-ok/10 border-ok/25 text-ok`.
- **Never use raw Tailwind palette colors** (`red-500`, `emerald-400`, …).

## Typography

- Page title: `font-expanded text-2xl font-extrabold tracking-tight text-fg`
  with an `eyebrow` line above it (mono micro-label, e.g. section context).
- Card/section headers: `text-sm font-semibold text-fg` or `eyebrow` class for
  data-table style headers. No icons inside headings.
- Sentence case everywhere. ALL CAPS only via `eyebrow` (mono micro-labels)
  and status badges.

## Components vocabulary

- Card: `rounded-md border border-edge bg-surface` (+ `p-5` typical). Nested
  panels: `bg-surface-alt`. No shadows on cards.
- Primary button: `rounded-md bg-accent px-4 py-2 text-sm font-semibold
  text-white transition-colors hover:bg-accent-hover` (dark text-page in dark
  mode is unnecessary — white works on ember in both themes).
- Secondary button: `rounded-md border border-edge bg-surface px-4 py-2
  text-sm font-medium text-fg-2 hover:border-fg-4 hover:text-fg`.
- Danger button: like secondary but `border-danger/30 text-danger
  hover:bg-danger/10`.
- Ghost icon button: `rounded-md p-2 text-fg-3 hover:bg-surface-alt
  hover:text-fg`.
- Input/textarea/select: `rounded-md border border-edge bg-input px-3 py-2
  text-sm text-fg placeholder-fg-4 focus:border-accent focus:outline-none`.
  Values that are machine data get `font-mono`.
- Status badge: see `src/components/StatusBadge.tsx` — mono 10px uppercase,
  dot + label, semantic color; active states use `animate-ember` on the dot.
- Empty state: centered, lucide icon `size-6 text-fg-4`, one plain sentence,
  optionally one action button. An empty screen invites the next action.

## Icons

`lucide-react` only (no Material Symbols). Default `size-4` inline with text,
`size-5` in nav/buttons. `strokeWidth={1.75}` for large decorative icons.

## Motion

- `animate-fade-in-up` for page/card entry (sparingly, once per view).
- `animate-ember` — live-activity glow (the signature; use exactly where
  something is running).
- `animate-soft-pulse` — opacity breathing for non-ember live dots.
- Everything else: `transition-colors` only. Respect reduced motion (handled
  in CSS).

## Copy

Plain verbs, sentence case: "New session", "Log out", "Create PR". Errors say
what went wrong and what to do next. Buttons keep the same name through the
whole flow.
