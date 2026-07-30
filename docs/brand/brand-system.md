# windylane brand system

windylane is a monochrome developer-tool brand for a serious saas control plane.

this file is the source of truth for dashboard styling. the brand board image
is visual reference only.

---

## core principles

- lowercase by default
- monochrome only
- professional, minimal, serious
- developer-first
- clean information density
- no decorative clutter
- no colorful gradients
- no emoji-based ui
- no playful illustration style
- no fake marketing filler
- no unnecessary animation
- no heavy component library

---

## brand voice

windylane should feel:

- reliable
- technical
- focused
- calm
- precise
- production-ready

avoid:

- hype-heavy copy
- toy-like ui
- exaggerated gradients
- flashy animations
- random decorative icons

---

## name usage

product name:

```txt
windylane
```

always write the product name lowercase in the dashboard unless required by a technical context.

domain target:

```txt
windylane.dev
```

slogan:

```txt
stay in flow.
```

current assets:

- brand board: `docs/brand/windylane-brand-board.png`
- dashboard logo: `apps/dashboard/public/brand/windylane-logo.png`
- dashboard mark: `apps/dashboard/public/brand/windylane-mark.png`

the logo and standalone mark must use the same smooth, monochrome two-node lane
emblem: one node at the upper-left start, one at the lower-right end, connected
by a rounded continuous path. do not substitute a legacy emblem or create a
second mark for the wordmark lockup.

---

## color system

use only the following tokens.

| token | hex | usage |
|---|---|---|
| ink | `#0d0d0d` | app background |
| graphite | `#1a1a1a` | cards, sidebar, panels |
| surface-muted | `#242424` | elevated surfaces, hover states |
| slate | `#404040` | borders, dividers, disabled states |
| mist | `#bfbfbf` | secondary text |
| paper | `#ffffff` | primary text, primary foreground |

css variables:

```css
:root {
  --wl-ink: #0d0d0d;
  --wl-graphite: #1a1a1a;
  --wl-surface-muted: #242424;
  --wl-slate: #404040;
  --wl-mist: #bfbfbf;
  --wl-paper: #ffffff;
}
```

do not introduce additional brand colors.

status colors should remain monochrome:

| status | style |
|---|---|
| success | paper text, subtle border |
| warning | mist text, dashed/slate border |
| error | paper text, stronger slate border, no red |
| pending | mist text, slate border |
| active | paper text, paper border or filled paper badge |
| inactive | mist text, slate border |

---

## typography

preferred typeface:

```txt
inter
```

fallback:

```css
font-family: inter, ui-sans-serif, system-ui, -apple-system, blinkmacsystemfont, "segoe ui", sans-serif;
```

rules:

- use lowercase labels where natural
- headings should be short
- no oversized hero text inside the dashboard
- tables should prioritize readability
- use medium/semibold for labels and headings
- avoid overly thin text for important ui

suggested scale:

| role | size |
|---|---|
| page title | 24px / 32px |
| section heading | 16px / 24px |
| body | 14px / 22px |
| metadata | 12px / 18px |
| table text | 13px / 20px |

---

## layout

dashboard layout:

- dark app background
- fixed or sticky left sidebar
- topbar/header for page context
- content width should feel spacious but dense
- use consistent vertical rhythm
- avoid huge empty hero sections
- use cards only when they group related information
- tables should be clean and compact

spacing scale:

| token | value |
|---|---|
| xs | 4px |
| sm | 8px |
| md | 12px |
| lg | 16px |
| xl | 24px |
| 2xl | 32px |

border radius:

| component | radius |
|---|---|
| buttons | 8px |
| cards | 14px |
| inputs | 8px |
| tables | 12px |
| badges | 999px |

borders:

```css
border: 1px solid var(--wl-slate);
```

use opacity or darker surfaces instead of new colors.

---

## logo direction

wordmark:

```txt
windylane
```

icon direction:

- abstract pipeline
- connected nodes
- queue/flow metaphor
- simple enough for favicon/sidebar
- monochrome only
- do not create a new colorful logo
- do not use emoji or mascot imagery

acceptable dashboard usage:

- sidebar brand mark
- top-left app identity
- favicon/app icon if practical
- small icon-only collapsed state if practical

---

## ui components

### sidebar

- graphite background
- slate border
- lowercase nav labels
- selected item uses surface-muted or paper outline
- no colorful active indicators
- compact icons only if already available or easy to create with css/svg

nav labels:

- overview
- tasks
- workers
- dlq
- webhooks
- webhook deliveries
- settings

### topbar

- transparent or ink background
- subtle bottom border
- page title lowercase
- optional short muted description
- no marketing copy

### cards

cards should use:

- graphite background
- slate border
- 14px radius
- compact title
- muted metadata
- clear action area

avoid:

- large gradient cards
- decorative graphics
- fake metrics

### buttons

primary button:

- paper background
- ink text
- subtle border
- no color accent

secondary button:

- transparent or graphite background
- paper text
- slate border

danger/destructive button:

- no red required
- use clear wording
- require confirmation for destructive actions
- border can be stronger slate/paper

### forms

- labels lowercase
- inputs graphite/surface-muted
- slate border
- paper text
- mist placeholder
- validation text concise
- do not expose api keys
- webhook secrets only visible once after create/rotation

### tables

- compact rows
- clear header
- slate dividers
- no zebra colors beyond graphite/surface-muted
- status badge in monochrome
- actions aligned right
- truncate long ids but allow copying if practical

### badges

- rounded full pill
- monochrome only
- border-based state
- small text
- no bright status colors

### empty states

empty states should be useful:

- state what is missing
- give one clear action
- avoid fake excitement

example:

```txt
no webhooks yet
create an endpoint to receive task lifecycle events.
```

### loading states

- simple skeletons or muted text
- no spinners unless already present
- no animations beyond subtle pulse if already implemented

### error states

- readable message
- stable layout
- clear next action
- no raw stack traces
- no secrets in errors

---

## dashboard page expectations

### tasks

- task list should look like an operational table
- status must be easy to scan
- id, queue, status, retry count, worker, timestamps
- detail page should show payload/result as formatted json

### webhooks

- endpoint list with name, url, active status, event types
- create page shows one-time secret after creation
- detail page supports update, delete/disable, rotate secret
- rotated secret visible once only

### webhook deliveries

- log-style table
- filters for endpoint, status, event type
- detail page shows payload, response status, response body, last error
- retry action for failed/retryable deliveries

---

## implementation rules

- do not redesign the product
- do not invent new colors
- do not add heavy ui libraries
- do not expose secrets to browser bundles
- keep server-only api client patterns
- keep components maintainable and readable
- keep dashboard accessible enough for keyboard and screen-reader basics
- preserve existing backend behavior
- run dashboard lint and build after changes

---

## ownership

the windylane brand system, logo direction, visual identity, and files under `docs/brand/` are not open source and are not licensed for reuse as a brand identity.

see [`../../trademark.md`](../../trademark.md).