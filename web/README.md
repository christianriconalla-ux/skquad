# skquad Web App

The SPA (React / Next.js 16, TypeScript). The primary interface for users,
optimised for simplicity: onboarding in a few clicks, and the Kanban board as
the centre of daily use.

- Design: [`docs/web-app-ux.md`](../docs/web-app-ux.md)

## Layout
```
web/
├── src/app/               # Next.js app router and global styles
├── src/lib/               # Browser API client helpers
└── package.json
```

## Current Implementation

- Provides the authenticated application shell with top-level navigation for
  squads, agents, tasks, registry, and admin areas.
- Represents dev-mode auth by calling the API without a token and future OIDC
  bearer auth through a locally stored token field.
- Uses `NEXT_PUBLIC_SKQUAD_API_BASE_URL` for browser API calls. The Helm chart
  defaults this to `/api/v1`, matching the chart ingress path split.
- Loads `/auth/me`, `/squads`, squad agents, squad boards, registry catalogs,
  selected agent permissions, squad access grants, selected agent chat history,
  platform metering summary, and platform audit with bounded loading, empty, and
  error states.
- Supports first-pass operational workflows for creating squads, updating squad
  mission text, adding agents, creating/rotating agent identities, creating
  tasks, moving tasks across Kanban statuses, reassigning tasks, deleting tasks,
  enqueueing user-to-agent consult messages, registering/deprecating registry
  entries, granting/revoking agent resource permissions, and creating/revoking
  squad access grants.
- Keeps richer visualizations, guided onboarding, drag and drop, live updates,
  and user-management settings as follow-up screens.

## Verification

```
npm audit --audit-level=moderate
npm run lint
NEXT_TELEMETRY_DISABLED=1 npm run build
```

The app uses ESLint 9 flat config via `eslint.config.mjs`; the old `next lint`
entrypoint is not used with the current Next.js version.
