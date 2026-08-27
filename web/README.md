# skquad Web App

The SPA (React / Next.js, TypeScript). The primary interface for users,
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

- Provides the first authenticated application shell with top-level navigation
  for squads, agents, tasks, registry, and admin areas.
- Represents dev-mode auth by calling the API without a token and future OIDC
  bearer auth through a locally stored token field.
- Uses `NEXT_PUBLIC_SKQUAD_API_BASE_URL` for browser API calls. The Helm chart
  defaults this to `/api/v1`, matching the chart ingress path split.
- Loads `/auth/me` and `/squads` into the shell, with bounded loading, empty,
  and error states.
- Keeps feature screens beyond the squad overview as placeholders for the next
  workflow slices.

## Verification

```
npm run lint
NEXT_TELEMETRY_DISABLED=1 npm run build
```
