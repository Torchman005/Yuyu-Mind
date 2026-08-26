# Yuyu-Mind2 UX Contract

This contract records durable frontend behavior for the Wails desktop companion shell. Business and architecture facts live in `AGENT.md`, `README.md`, and `docs/DEVELOPMENT-NOTES.md`; this file records observable UI ownership and state rules.

## Canonical UI Map

| Capability | Canonical owner | Source of truth | Allowed variants | Verification |
|---|---|---|---|---|
| Select/Listbox | Native select for low-risk filters | `premium-ui.json` + `DESIGN.md` | native | premium audit + browser smoke |
| Scrollbar | Global application stylesheet | `frontend/src/App.css` | geometry exceptions only | premium audit + visual smoke |
| Form | Screen-owned lightweight forms | `frontend/src/App.tsx` / `frontend/src/components/AppShell.tsx` | chat composer / JSON editor | TypeScript + premium audit |

## Workflow Rules

- Chat composer uses app-owned validation: empty text submissions are disabled, and the form uses `noValidate`.
- Plugin and settings JSON editors are fixed-size textareas with internal scrolling.
- Logs use a native select because popup geometry is not critical to the workflow.
- Task/code-review decisions keep destructive actions visually separate from routine actions.
- Error states are inline and persistent rather than browser dialogs.
