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
  columns; detail view; add/edit form; reveal/copy; extend/activate/deactivate;
  each account has its own currency (`account.currency`; anything that adds amounts up passes the
  accounts through `withUsdAmounts` in `money.ts` first);
  **right-click (long-press on a phone) a row → Duplicate** opens the new-account form filled from that account
  (including its password, fetched like a normal reveal, so it appears in the audit log).
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
  The emails the *server* sends are worded separately, in both languages, in
  `internal/report/messages.go`.

## Project layout

- `src/api.ts` — typed client for the Go API.
- `src/i18n.ts`, `src/locales/{en,ar}.json` — translations.
- `src/settingsSearch.ts`, `src/components/SettingsSearch.tsx` — the Settings search: a curated index of every section/setting (EN + AR at once, with synonyms in `settingsSearch.kw.*`). Add new settings there and give their section an `id="s-…"` so the search can jump to it.
- `src/celebrate.ts` + `public/sounds/cha-ching.mp3` — the small "Mark as paid" reward: the bundled sound plus green `$` confetti (falls back to a synthesised sound if the mp3 can't play; "reduce motion" skips the confetti). Replace the mp3 (keep the name) to change the sound. Two per-browser switches (**Settings → Admin → "Turn off annoying easter egg?"**, `CelebrationSettings.tsx`, stored in localStorage) turn the sound and the confetti off.
- `src/components/Pagination.tsx` — `usePagination(list, resetKey)` + `<Pagination>`: the page footer used by every long list (accounts, authenticator, users, payment and password history). Page size (10/25/50/100) is remembered in the browser; pass a `resetKey` built from the list's filters/sort so it returns to page 1 when they change. Exports always use the full filtered list, not the current page.
- `src/components/` — shared UI (Layout, Sidebar, Button, Combobox, …).
- `src/pages/` — routed pages (Home, Accounts, Finance*, Settings*, Health, …).
- `src/account-ui.tsx` — account row / detail / form building blocks.
- `src/strength.ts`, `src/pwned.ts`, `src/health.ts` — password strength, HIBP,
  and the Vault Health audit (all computed client-side after decryption).
