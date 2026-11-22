# Design

Visual system of the Interlinea prototype (`prototype/src/styles.css`).
Category standard, at the craft level of memoQ 11 and Phrase TMS: a working
tool first, distinctive only through precision.

## Scene

Translators at a desk in daylight, long sessions, often two monitors. Light
theme. Dense but calm.

## Colour

Restrained: cool neutrals, one blue accent for primary actions, selection and
current location, semantic colours only where they carry meaning.

| Token | Value | Use |
|---|---|---|
| `--bg` | `#f3f5f8` | App ground |
| `--rail` | `#e8ecf1` | Navigation rail (second neutral layer) |
| `--surface` | `#ffffff` | Panels, panes, tables |
| `--surface-2` | `#f7f9fb` | Inset areas, table headers, terms |
| `--border` / `--border-strong` | `#d8dde4` / `#b9c1cc` | Dividers / control borders |
| `--text` / `--text-2` / `--text-3` | `#1a1f26` / `#4c5562` / `#66707c` | Primary / secondary / tertiary text (all at least 4.5:1 on white) |
| `--accent` | `#1f5bd6` | Primary buttons, focus, selection |
| `--accent-weak` / `--accent-text` | `#e8effd` / `#1a4aae` | Selected rows, links, quiet buttons |
| `--ok`, `--warn`, `--danger` | `#17703f`, `#7f5300`, `#aa2219` | Approved / proposed / forbidden and conflicts, each on its own tinted background |
| `--server` | `#4a3aa8` on `#eeebfb` | "Server" location badge (local is neutral) |

## Type

Platform UI font (Segoe UI Variable on Windows, San Francisco on macOS,
system sans on Linux, Noto for CJK), so the web prototype matches the later
Qt desktop client. Fixed rem scale: 12, 13, 14 (body), 16, 18, 22 px.
Weights 400, 550, 600, 650. Tabular numerals in counts and tables.

## Shape and depth

Controls 6 px radius, panels 10 px. Elevation by border only; shadows only on
floating elements (toast, sticky merge bar). Pills for badges and tags.

## Components

Standard controls throughout: buttons (primary, default, quiet, small),
inputs, selects, checkboxes, radios, segmented control, tabs, tables, lists.
Every interactive element has hover, focus-visible, active and disabled
states. Language codes appear as small outlined chips with the full language
name as tooltip; text in a language carries `lang`.

## Motion

Only state: 120 to 180 ms transitions on hover and selection, one toast
entrance. Disabled under `prefers-reduced-motion`.

## Accessibility

Skip link, focus moves to the page heading on navigation, live region for
toasts and merge status, forced-colors (Windows high contrast) styles for
selection and badges.
