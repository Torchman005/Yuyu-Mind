---
version: alpha
name: "Yuyu-Mind2"
description: "A soft glass anime desktop-companion shell for chat, Live2D presence, plugins, tasks, logs, and local model controls."
colors:
  background: "#FBF7FB"
  surface: "rgba(255,255,255,0.72)"
  surfaceSoft: "#F8EDF4"
  primary: "#C978AD"
  secondary: "#78A9D6"
  mint: "#82BFA9"
  warning: "#C99254"
  danger: "#C65C74"
  text: "#33283D"
  textMuted: "#756D7E"
  border: "rgba(217,200,214,0.76)"
typography:
  body:
    fontFamily: "\"Nunito\", \"Segoe UI\", system-ui, -apple-system, sans-serif"
  display:
    fontFamily: "\"Nunito\", \"Segoe UI\", system-ui, -apple-system, sans-serif"
  mono:
    fontFamily: "\"Cascadia Mono\", \"JetBrains Mono\", Consolas, monospace"
rounded:
  DEFAULT: "12px"
  sm: "8px"
  md: "12px"
  lg: "18px"
  pill: "999px"
spacing:
  shell-gap: "14px"
  panel-padding: "22px"
  dense-gap: "8px"
  page-max: "1200px"
components:
  button: { }
  card: { }
  sidebar: { }
  chat-message: { }
  code-diff: { }
---

# Yuyu-Mind2 Design System

## Overview

### Creative North Star

Yuyu-Mind2 should feel like a soft glass desktop companion shell: gentle anime character energy, translucent panels, and clear local-agent controls. The interface can be playful, but it must never hide task state, approvals, logs, or model errors.

### Product Context And Register

- **Audience and primary job:** A developer/operator chats with Yuyu, manages local model services, reviews agent work, and checks plugins/tasks/logs in a Wails desktop shell.
- **Target markets and evidence:** The current product copy is Chinese and the repository describes a personal desktop companion. No Japan-specific market or regulated commerce flow is in scope.
- **Locale and language policy:** Owned UI copy is Simplified Chinese with technical identifiers left in English when they are literal API/plugin/status names.
- **Usage scene:** Desktop-first, repeated during coding or content creation, with occasional transparent pet mode overlaying the desktop.
- **Register:** Hybrid. Chat and Live2D surfaces carry expressive brand personality; plugins, tasks, settings, and logs stay product-dense and scannable.
- **Memorable signature:** Frosted-glass companion panels: translucent surfaces, subtle reflected highlights, and low-saturation status chips tied to Yuyu's modes.
- **Restraint:** Keep code review, logs, config, and task approvals quiet, high-contrast, and spatially stable.
- **Anti-references:** Avoid all-black IDE skins, single-hue blue dashboards, beige editorial landing pages, and decorative anime clichés that reduce legibility.
- **Token ownership/runtime mapping:** Existing runtime CSS variables in `frontend/src/App.css` remain canonical. This file mirrors accepted durable tokens and explains their roles.

## Colors

The default application theme is light, low-saturation, and glassy. `background` and `surfaceSoft` carry warmth without candy intensity; `primary` is a muted rose for brand/action emphasis; `secondary` is a softened blue for information and navigation; `mint`, `warning`, and `danger` stay semantic. Text remains aubergine-toned instead of pure black for softer contrast without losing readability.

## Typography

Nunito is already bundled and remains the body/display face because its rounded forms match the desktop-pet identity. Chinese text falls through to Segoe UI and system CJK fonts. Monospace areas use Cascadia Mono/JetBrains Mono/Consolas for logs, diffs, and config.

## Layout

The desktop detail mode uses a fixed sidebar plus one flexible content pane. Chat and log feeds own their own scroll regions; forms and settings keep natural document scrolling. Pet mode remains transparent and minimal, with controls outside the Live2D hit area.

## Elevation & Depth

Depth comes from translucent surfaces, backdrop blur, reflected top edges, and restrained shadows. Static utility panels use glass borders; important interactive cards may lift slightly on hover. Dark code/diff surfaces are allowed only for technical text blocks where syntax contrast matters.

## Shapes

Controls use pill radius when they are short actions or chips. Panels and cards use 12-18px radii; dense log rows and diff rows use 8px. Nested cards should be avoided unless the inner surface represents a separate repeated object.

## Components

### Foundational Visual States

Every button has hover, focus-visible, active/pressed, disabled, and busy-stable geometry. Scrollbars inherit a global themed baseline. Textareas are not manually resizable; long content scrolls internally.

### Buttons And Actions

Brand actions use `primary`; neutral actions use white or soft surfaces; destructive actions use `danger` and remain visually separated from routine controls. Icon-like labels may be used only when the symbol is already familiar in this product.

### Navigation And Data Display

Sidebar navigation uses colored active state plus shape, not color alone. Cards in plugins/tasks expose name, status, and next action without depending on hover-only affordances. Code review lists preserve filenames in monospace and allow full paths through wrapping or ellipsis.

### Forms And Overlays

Configuration editors use fixed-height textareas with internal scrolling. Native select is acceptable for the logs level filter because platform-owned popup geometry is not business-critical. Errors are inline, persistent, and specific.

### Iconography

No external icon set is installed. Use concise glyphs or text labels already present in the app; do not introduce a second icon language unless an icon package is deliberately adopted later.

### Motion

Motion is small and stateful: hover lift, active press, and Live2D presence. Respect reduced motion by disabling decorative transitions and animations outside the avatar's own rendering.

### Content And Data Visualization

UI copy is friendly Simplified Chinese, direct, and task-oriented. Model/plugin/status identifiers may remain literal English for debugging accuracy.

## Do's And Don'ts

- **Do:** Use low-saturation accents to clarify mood, status, and navigation.
- **Do:** Preserve glass clarity with blur, reflected edges, and readable contrast.
- **Do:** Keep task approvals, logs, and code review layouts stable while data changes.
- **Don't:** Return the web tool pages to a mostly black theme.
- **Don't:** Use rainbow/candy copy or overly saturated gradients for routine app chrome.
- **Don't:** Hide scrollbars, use browser dialogs, or make non-button elements responsible for core actions.
