# Account Manager

A credential & subscription vault designed for personal use or a tech-support team.

- **Single self-contained binary:** Go backend with the React frontend embedded
  and an **embedded SQLite** database. No separate database, no Docker — download
  one file and run it.
- **Web:** React + TypeScript + Vite (English/Arabic, RTL).

See [`DEPLOY.md`](DEPLOY.md) to install/run it (personal or server),
[`THREAT_MODEL.md`](THREAT_MODEL.md) for the security posture, and
[`AUDIT_LOG.md`](AUDIT_LOG.md) for how to read the log's action codes.

> **Security note:** encryption today is **server-side envelope encryption**
> (a master key kept outside the DB) — protection against a DB-only leak, not yet
> a zero-knowledge system. The client-side "inner lock" is planned. Details in the
> threat model.

## Running it (as a user)

**Windows:** run the installer (`account-manager-setup.exe`). It installs per-user,
adds a Start-Menu shortcut, and launches the app in the background with a
system-tray icon (right-click → Open / Quit). Your browser opens automatically.

**macOS / Linux:** download the binary (or the macOS `.dmg`) and run it:

```bash
./account-manager          # serves on http://localhost:8080
```

Then open **http://localhost:8080**. **First run happens entirely in the browser:**
you choose a vault passphrase (which protects the master key) and create the first
admin account. On later starts you enter the passphrase to unlock.

Only one instance runs at a time (launching it again just opens the browser), and
there's nothing to install or migrate.

**Where your data lives** (DB, master key, and backups — the folder to back up):

| How you run it | Data folder |
| --- | --- |
| **Windows installer** | `%LocalAppData%\Programs\Account Manager\data` |
| **macOS `.dmg`** | `~/.account-manager` |
| **Binary you run yourself** (Linux, or a raw exe) | `./data` — **relative to your current directory when you launch it**, not next to the binary |

> So for the raw binary, run it from a folder you'll keep (e.g. `~/account-manager/`)
> and always start it from that same place — or set an absolute `DATA_DIR` (see
> [Configuration](#configuration)) to pin the location. The installers/`.dmg` handle
> this for you.

### Security features

- **Two-factor auth (TOTP)** for admin login — enable per admin under **Settings → Admin** (+ backup codes).
- **Audit log** — who revealed / changed / exported / restored what, with client IP (**Settings → Logs**). Action codes are explained in [`AUDIT_LOG.md`](AUDIT_LOG.md).
- **Password history** — previous passwords per account, kept and revealable on the account's **Password History** tab.
- **Payment history** — a ledger of actual payments per account. **Mark as paid** opens a prefilled form (today's date + the account's amount) you confirm or adjust, which records the payment and rolls the next-payment date forward; you can also add/edit/delete entries manually with amount, currency, note, and an optional linked invoice. See the account's **Payments** tab, or **Finance → Payments** for a filterable, exportable view across all accounts (export optionally bundles the invoice files as a `.zip`).
- **Account 2FA codes** — store a service's 2FA secret (paste the key/`otpauth://` URI **or scan the QR with your camera**) and generate its codes in-app (the "One-time password" card on the account's **General** tab).
- **Breach monitoring** — scheduled Have I Been Pwned check of stored passwords (**Settings → Configuration**), flagging breached ones. Uses k-anonymity — only a short hash prefix is sent.
- **Clipboard auto-clear** after copying a secret (configurable; **Settings → Configuration**). Note: this wipes the *active* clipboard, but a browser cannot clear **Windows Clipboard History (Win+V)** or cloud clipboard sync — disable/clear those at the OS level if that matters.
- Login rate-limiting + lockout, encrypted attachments, and in-app backups.
- **Update checks** — the app notifies you in-app when a new release is available and (on desktop) can one-click update; downloads are **SHA256-verified** against the release before running. Toggle in **Settings → Configuration**. See [`DEPLOY.md`](DEPLOY.md#updating).

### Where it listens

The app only listens on a host:port; a reverse proxy in front maps a domain and
handles HTTPS. Configure it in **Settings → Deployment**:

- **Personal:** localhost only (default `localhost:8080`) — no TLS needed.
- **Server:** "Accessible on the network" binds `0.0.0.0`; map your domain to it
  with your existing reverse proxy. `HOST` / `PORT` env vars override for the
  initial bootstrap. Changes apply on restart.

## Configuration

Everything has sensible defaults; see [`.env.example`](.env.example). A personal
install needs no configuration. Key points:

- **`MASTER_KEY`** — the encryption key. For an **unattended server**, set it in
  the environment (generate one with `go run ./cmd/api -gen-key`). If unset, the
  app uses a passphrase-sealed key file (`data/master.key`); you set that
  passphrase in the browser on first run and enter it (in the browser) to unlock
  on later starts. **Back this up** — losing it (with no sealed-file fallback)
  means permanent data loss.
- **`JWT_SECRET`** — auto-persists to `data/jwt.secret` if unset.
- **`DATA_DIR`** (default `./data`) — holds the DB, key file, and backups. It's
  relative to the working directory, so run the binary from a consistent place or
  set an absolute path.

### Operator commands

Run these against the **same data dir the app uses** (set `DATA_DIR`, or run from the
same folder). They exit immediately (they don't start the server). Per install:

- **Windows (installer):** open a terminal in `%LocalAppData%\Programs\Account Manager`
  and run `.\account-manager.exe -reset-admin …` (data is in `.\data` there).
- **macOS (`.dmg`):** the binary is **inside the app bundle**, and the data dir is
  `~/.account-manager`:
  ```bash
  DATA_DIR="$HOME/.account-manager" "/Applications/Account Manager.app/Contents/Resources/account-manager" -reset-admin
  ```
- **Linux / raw binary:** run `./account-manager …` from the folder that holds its
  `data/` (or set `DATA_DIR` to an absolute path).

The examples below use the Unix `./account-manager` form. **On Windows, replace
`./account-manager` with `account-manager.exe` in Command Prompt, or
`.\account-manager.exe` in PowerShell** For example:

```bat
account-manager.exe -reset-admin -email you@example.com -reset-password
```

**Key management:**
```bash
./account-manager -gen-key            # print a fresh base64 MASTER_KEY (server mode)
./account-manager -seal-master-key    # move an env MASTER_KEY into a passphrase file
./account-manager -change-passphrase  # change the sealing passphrase (same key)
```

**Admin recovery** (for a lockout — forgotten password or lost authenticator).
Listing admins and clearing 2FA need only DB access; a **password reset also asks
for the vault passphrase** (proof of ownership, so a stolen DB file alone can't
mint a working login). In `MASTER_KEY` server mode the env key is that proof, so
no prompt:
```bash
./account-manager -reset-admin                                   # list admin emails
./account-manager -reset-admin -email you@example.com -reset-password   # prompts for vault passphrase, then a new password
./account-manager -reset-admin -email you@example.com -clear-mfa        # disable & clear their 2FA
# (combine -reset-password and -clear-mfa to do both)
```

As a last resort, if *every* admin is locked out and recovery above isn't enough,
removing all admins reopens the first-run register flow (data is encrypted with
the master key, not passwords, so it stays intact):
```sql
-- sqlite3 data/account-manager.db
DELETE FROM admin;
```

> Full recovery matrix — forgotten password, lost 2FA, forgotten **passphrase**
> (the one unrecoverable case), and changing the passphrase — is in
> [`DEPLOY.md` → Disaster & recovery](DEPLOY.md#disaster--recovery).

## Developing

Requires **Go** and **Node.js** (no Docker, no database to install).

```bash
# Backend + embedded DB on :8080
go run ./cmd/api

# Frontend with hot-reload on :5173 (proxies /api -> :8080)
cd web && npm install && npm run dev
```

Use **:5173** while developing (live frontend). The Go server serves the *last
built* frontend on :8080; a backend change needs a `go run` restart.

### Building the single binary

```bash
cd web && npm run build      # builds the SPA into internal/webui/dist
cd .. && go build ./cmd/api  # produces the self-contained binary
```

Windows and Linux are pure Go (modernc SQLite, **no CGO**) and cross-compile from
anywhere with `GOOS`/`GOARCH`. The **macOS** build needs **CGO** (its menu-bar tray
uses Cocoa), so it's built on a Mac (see the release workflow).

## Repository layout

```
cmd/api            entrypoint (+ operator flags: -gen-key / -seal-master-key / -change-passphrase / -reset-admin)
internal/
  api              HTTP handlers, routes, middleware, rate limiting
  auth             JWT (session + MFA-scoped tokens)
  backup           scheduled SQLite snapshot backups
  breach           scheduled Have I Been Pwned monitoring
  config           env-based config
  crypto           Argon2id, AES-256-GCM envelope encryption, key sealing
  db               SQLite open + schema.sql + versioned migrations
  rates            daily exchange-rate fetch (open.er-api.com)
  store            data access (database/sql)
  totp             RFC 6238 TOTP + backup codes
  update           GitHub release check + checksum-verified self-update
  webui            embeds the built React SPA
web                React client (see web/README.md)
```

## Disaster recovery

Full backups are built in (**Settings → Configuration → Backups**). A backup is a
gzipped **SQLite snapshot** of the whole database — which **includes attachments**
(they're stored in the DB). Secrets and attachments stay **encrypted** in the
backup, so a backup is only usable together with the master key.

**The one rule that matters:** recovering the vault needs **two** things — a
**backup file** *and* the **master key**. A backup without the key is
unrecoverable ciphertext. **Store the master key separately** from the backups.
Losing it = permanent data loss.

- **Schedule & retention:** Off / Daily / Weekly, a time of day, and how many to
  keep. The server runs them automatically and prunes old ones.
- **Manual:** "Back up now" creates one immediately.
- **Storage:** written to `BACKUP_DIR` (default `data/backups`) as
  `backup-YYYYMMDD-HHMMSS.db.gz`, on the **same host** — use **Download** (or
  point `BACKUP_DIR` at a synced/off-site location) to keep copies off-machine.

### Restore

In-app: **Settings → Configuration → Backups → Restore** on a listed backup. This
**replaces all current data** with the backup and signs everyone out (confirm
required). The running server must use the **same master key** as when the backup
was made. Restore copies the snapshot in without a restart.

## Notes

- Outbound network calls (no keys or personal data sent):
  - the daily USD exchange-rate fetch (`open.er-api.com`);
  - the Have I Been Pwned range API (`api.pwnedpasswords.com`, k-anonymity — only a
    5-char SHA-1 prefix is sent), when breach monitoring is enabled;
  - the GitHub releases check for app updates (`api.github.com`), when update checks
    are on — plus the on-demand download of a release from GitHub when you apply an
    update. Both toggle off in **Settings → Configuration → Updates**.
- Every new user-facing web string must have English **and** Arabic translations
  (see `web/src/locales`).

## Credits

- App icon (Security Shield) by [Icons8](https://icons8.com).

## License

Licensed under the **[Apache License 2.0](LICENSE)** — free to use, modify, and
distribute, including commercially, with an explicit patent grant. See the
[`LICENSE`](LICENSE) file for the full terms.
