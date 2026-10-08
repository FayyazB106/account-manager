# Account Manager — Web client

React + TypeScript + Vite front-end for the Account Manager vault. Talks to the
Go API (default `http://localhost:8080`).

## Run

```bash
npm install
npm run dev        # Vite dev server, default http://localhost:5173
```

The API base URL is `BASE` in [`src/api.ts`](src/api.ts). The backend must be
running (see the repo root README).

Useful scripts:

```bash
npm run build      # type-check (tsc) + production build
npx tsc --noEmit   # type-check only
```

## What's here

- **Auth**: browser-based first-run setup + unlock; login (+ optional TOTP). The
  access token is **in memory only** (never in web storage); a long-lived refresh
  lives in an httpOnly cookie and is exchanged silently on startup / 401.
- **Accounts**: list with search + status/payment/rotation filters + sortable
  columns; detail view; add/edit form; reveal/copy; extend/activate/deactivate.
- **Account 2FA**: store a service's TOTP secret (paste key/URI or **scan the QR
  with the camera**) and show live codes.
- **Categories, Users (people)/seats, Admins** management under Settings.
- **Vault Health** (weak/reused/old + Have I Been Pwned check), **Finance**
  (accounts table with filters, calendar, monthly-cost graph), **dashboard** tiles.
- **Updates**: in-app "update available" banner + one-click update (desktop).
- **Attachments** (upload/download/export), **Excel export**, **organization
  branding**, **currencies**.
- **i18n**: English + Arabic via react-i18next, RTL layout, localized
  dates/numbers. See the memory rule — every new user-facing string needs `en`
  **and** `ar` keys in [`src/locales`](src/locales).

## Project layout

- `src/api.ts` — typed client for the Go API.
- `src/i18n.ts`, `src/locales/{en,ar}.json` — translations.
- `src/components/` — shared UI (Layout, Sidebar, Button, Combobox, …).
- `src/pages/` — routed pages (Home, Accounts, Finance*, Settings*, Health, …).
- `src/account-ui.tsx` — account row / detail / form building blocks.
- `src/strength.ts`, `src/pwned.ts`, `src/health.ts` — password strength, HIBP,
  and the Vault Health audit (all computed client-side after decryption).
