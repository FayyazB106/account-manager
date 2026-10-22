# Account Manager — Install & Deploy

Account Manager is a **single self-contained binary** (the web UI is baked in) backed
by an embedded SQLite database. There is nothing else to install — no Node, no
Postgres, no Docker. Two ways to run it:

- **Personal** — on your own machine, reachable only at `http://localhost:8080`.
- **Server** — on a shared host, reachable at a domain, with TLS terminated by
  the host's reverse proxy.

Downloads are on the project's **GitHub Releases** page:
- Windows — `account-manager-setup.exe` (installer; desktop use). For a **Windows
  server**, run it as a service using the exe the installer drops — see
  [Windows server](#windows-server).
- Linux — `account-manager-linux-amd64.tar.gz`
- macOS — `account-manager-macos.dmg` (universal; unsigned)

---

## Personal mode (your own PC)

The app serves at **http://localhost:8080**. **First-run setup happens in the
browser:** you choose a **passphrase** that protects the master key (which encrypts
every stored secret) and create the first admin account. On later starts you enter
the passphrase (in the browser) to unlock. Only one instance runs at a time —
launching the app again just opens the browser to the running one.

**Windows** — run `account-manager-setup.exe`. It's a per-user install (no admin
needed) with a setup wizard, a Start-Menu shortcut, optional desktop and "start
when I sign in" shortcuts, and an uninstaller. Launch it from the Start Menu — the
server starts **in the background with a system-tray icon** (near the clock:
right-click → **Open** / **Quit**) and your browser opens automatically. If you
change the port in **Settings → Deployment** and restart, the tray icon opens the
new port.

**Linux**
```bash
tar -xzf account-manager-linux-amd64.tar.gz
./account-manager-linux-amd64        # serves :8080 (do first-run setup in the browser)
```

**macOS**
1. Open the `.dmg` and drag **Account Manager.app** to Applications.
2. It's **unsigned / not notarized** (no paid Apple Developer account), so the first
   launch is blocked by Gatekeeper ("Apple could not verify…"). Approve it once —
   any one of:
   - **System Settings → Privacy & Security →** scroll to *"Account Manager was
     blocked"* **→ Open Anyway** (the reliable path on macOS 15 Sequoia).
   - Or right-click the app → **Open** → **Open** (macOS 12–14).
   - Or Terminal: `xattr -dr com.apple.quarantine "/Applications/Account Manager.app"`
   This is one-time per install.
3. It then runs in the background with a **menu-bar icon** (top-right: click →
   **Open** / **Quit**) and your browser opens at http://localhost:8080 — do the
   first-run setup there. No Terminal window; quit from the menu-bar icon. Data is
   stored in `~/.account-manager`.

> **Back up your data.** Everything (accounts, attachments, settings) is in the
> data directory. Losing the **master key** — and its passphrase — means the
> encrypted data is unrecoverable.

---

## Server mode

Run the binary on the host and let a reverse proxy map a domain to it and
terminate TLS.

1. **Copy the binary** to the host (e.g. the Linux `.tar.gz`).
2. **Set a master key** so it starts unattended (no passphrase prompt):
   ```bash
   ./account-manager-linux-amd64 -gen-key      # prints a random MASTER_KEY
   ```
   Put that value in the environment (systemd `EnvironmentFile`, or a `.env` beside
   the binary). Real env vars override a `.env`.
3. **Bind to the network.** For the very first boot before the UI is reachable,
   start it with `HOST=0.0.0.0`. After you can log in, set it permanently in
   **Settings → Deployment** ("Accessible on the network"), plus the **Public URL**
   (your domain) so links/cookies are correct. Changes there apply on restart.
4. **Create the first admin.** With `MASTER_KEY` set the vault starts unlocked, so the browser
   setup (passphrase) is skipped. Open the app once: on a fresh install it shows a
   **Create the first admin** form (this only appears while no admin exists). Do this right
   away, ideally over an SSH tunnel (`ssh -L 8080:localhost:8080 user@host`, then
   `http://localhost:8080`) so nobody else can reach the app before an admin exists.
5. **Map the domain** (e.g. `accounts.example.com`) to `http://<host>:8080` in your
   reverse proxy. The proxy should terminate TLS and
   pass `X-Forwarded-Proto: https` and `X-Forwarded-For` (so secure cookies, HSTS,
   and audit-log client IPs are correct).

**systemd example** (`/etc/systemd/system/account-manager.service`):
```ini
[Unit]
Description=Account Manager
After=network.target

[Service]
WorkingDirectory=/opt/account-manager
ExecStart=/opt/account-manager/account-manager-linux-amd64
EnvironmentFile=/opt/account-manager/.env
Restart=on-failure
User=accountmgr

[Install]
WantedBy=multi-user.target
```
```bash
sudo systemctl daemon-reload && sudo systemctl enable --now account-manager
```

### Updating a Linux server

A server never updates itself: the in-app banner only tells you a release exists. To update,
use [`scripts/update.sh`](scripts/update.sh) (copy it to the server once):

```bash
sudo bash update.sh v1.1.0                 # download that release, verify it, install it
sudo bash update.sh latest                 # the newest release
sudo bash update.sh ./account-manager-linux-amd64.tar.gz   # a tarball you copied over
```

It verifies the download against the release's `SHA256SUMS`, backs up the binary and the
`data/` folder next to them (`*.bak-<timestamp>` — delete them when you're happy), stops the
service, installs the new binary, starts it and checks that it answers. If it doesn't come up
it restores the old binary and data and starts the old version. Your `.env` and `data/` are
never overwritten. The database upgrades itself on first start, and older versions can't open
an upgraded database — which is why it keeps the backup. Defaults can be changed with
`INSTALL_DIR`, `SERVICE`, `SERVICE_USER` and `PORT` environment variables (see the top of the script).

### Windows server

The Windows **installer** and its tray icon are a *desktop* convenience — a headless
server has no interactive session for the tray, and you don't want it waiting on a
browser unlock after a reboot. Run it as a **service** instead:

1. Get the exe. Either run the installer and use the exe it drops at
   `%LocalAppData%\Programs\Account Manager\account-manager.exe`, or just copy that
   exe to a stable service-owned path (e.g. `C:\Services\AccountManager\`). You do
   **not** use `launch.vbs`, the tray, or the Start-Menu shortcuts on a server.
2. Set a master key so it boots unattended (no browser unlock):
   ```
   account-manager.exe -gen-key
   ```
3. Install it as a service with **NSSM**, setting the environment (`MASTER_KEY`,
   `DATA_DIR`, `PORT`) so it starts on boot without anyone logging in:
   ```
   nssm install AccountManager "C:\Services\AccountManager\account-manager.exe"
   nssm set AccountManager AppEnvironmentExtra MASTER_KEY=<key> DATA_DIR=C:\Services\AccountManager\data PORT=8080
   nssm start AccountManager
   ```
4. Put a reverse proxy (IIS / nginx / Caddy) in front for the domain + TLS, exactly
   as in the Linux steps above — keep the app **Localhost only** and set **Public
   URL** to your domain.

So on a server the login/vault behaviour is identical; only the wrapper changes:
`MASTER_KEY` env + a service (NSSM) + a reverse proxy, instead of the desktop
installer/tray/browser-unlock flow.

### TLS
The recommended setup terminates TLS at the **reverse proxy** (real,
auto-renewing certificate). If you'd rather serve HTTPS directly (e.g. LAN with
your own cert), set `TLS_CERT` and `TLS_KEY` to PEM files and the app serves HTTPS
itself. Localhost/personal mode stays HTTP (already a browser secure context).

---

## Environment variables

All optional except where noted; sensible defaults shown.

| Var | Purpose | Default |
| --- | --- | --- |
| `MASTER_KEY` | Encrypts every secret. Set for **unattended/server** mode. | *(sealed file + passphrase if unset)* |
| `MASTER_PASSPHRASE` | Supplies the passphrase non-interactively (CI/dev only). | *(prompted)* |
| `JWT_SECRET` | Session-signing secret. | auto-created at `data/jwt.secret` |
| `DATA_DIR` | DB, key file, backups live here. | `./data` |
| `DB_PATH` | SQLite file. | `<DATA_DIR>/account-manager.db` |
| `KEY_PATH` | Sealed master-key file. | `<DATA_DIR>/master.key` |
| `BACKUP_DIR` | Scheduled backup location. | `<DATA_DIR>/backups` |
| `HOST` | Bind host for the initial bootstrap (`0.0.0.0` = all). | from Settings → Deployment |
| `PORT` | Listen port. | `8080` |
| `TLS_CERT` / `TLS_KEY` | PEM cert + key to serve HTTPS directly (both required). | *(unset → HTTP)* |

## Operator commands (run the binary with a flag)

- `-gen-key` — print a fresh random `MASTER_KEY` for the environment.
- `-seal-master-key` — move an env `MASTER_KEY` into a passphrase-sealed file
  (personal mode). No data is re-encrypted.
- `-change-passphrase` — re-seal the same key under a new passphrase.
- `-reset-admin [-email X] [-reset-password] [-clear-mfa]` — recover a lockout
  without hand-editing SQL. Listing admins and `-clear-mfa` need only DB access;
  `-reset-password` also prompts for the vault passphrase (or uses `MASTER_KEY` in
  server mode) as proof of ownership.

## Backups & upgrading

- **Backups:** schedule + retention in **Settings → Configuration → Backups**;
  files land in `<DATA_DIR>/backups` and can be downloaded/restored from the UI. A
  backup is a full snapshot (attachments included).
- **Upgrading (manual / server):** stop the app, replace the binary, start it
  again. The database migrates automatically (schema is versioned). **Back up
  first.**

## Updating

The app checks GitHub for new releases and shows an **"Update available"** banner
in-app (and under **Settings → Configuration → Updates**, with the current version
and a "Check now" button).

- **Detection:** a daily background check of the project's GitHub releases. On by
  default; turn it off with the **"Check for updates automatically"** toggle. The
  source repo is **fixed at build time** to the official project repo — it is
  deliberately *not* runtime-configurable, so nothing can redirect the updater
  (which downloads + runs an installer) at an untrusted release. The check is
  best-effort — if GitHub is unreachable, it silently reports "no update".
- **Applying (desktop, one-click):** click **Update now**. The confirm dialog has a **"Back up my data first"** box, ticked by default: the app makes a backup (the same as **Settings → Configuration → Backups**) before doing anything else, and if the backup fails the update does not start. Then the app downloads the
  right file for your OS, **verifies its SHA256** against the release's `SHA256SUMS`
  before running it, then:
  - **Windows** → runs the new installer **silently** (a progress bar, no wizard
    clicks), upgrades in place, and relaunches — your desktop/startup shortcut
    choices are preserved.
  - **macOS** → opens the new `.dmg` (drag to replace, then reopen).
- **Servers / Linux:** **notify only** — the banner links to the release, but the
  app never self-restarts a service. On Linux, run [`scripts/update.sh`](scripts/update.sh)
  (backs up, verifies the download, swaps the binary, checks it starts, rolls back if not) —
  see [Updating a Linux server](#updating-a-linux-server).

> The download is always checksum-verified before it's executed, so a tampered or
> interrupted download is rejected rather than run.

## Uninstalling

Your **data is preserved** by every uninstall path below (so you can reinstall
without losing the vault). To wipe it too, delete the data directory as noted.

- **Windows (installer):** **Start Menu → Uninstall Account Manager**, or
  **Settings → Apps → Account Manager → Uninstall**. The uninstaller stops the
  running app first (so nothing is left running). Your vault stays in the `data`
  folder inside the install directory; to remove everything, also delete
  `%LocalAppData%\Programs\Account Manager` after uninstalling. *(To open that
  folder: paste `%LocalAppData%\Programs\Account Manager` into the File Explorer
  address bar and press Enter.)*
- **macOS:** quit from the **menu-bar icon**, then drag **Account Manager.app**
  from Applications to the Trash. Data lives in `~/.account-manager`; delete that
  folder to remove the vault too. *(It's a hidden folder, so Spotlight won't find
  it — open it with `open ~/.account-manager` in Terminal, or in Finder press
  ⇧⌘G and enter `~/.account-manager`.)*
- **Linux:** stop it (`Ctrl+C`, or `systemctl disable --now account-manager` if you
  set up the service and remove its unit file), then delete the binary. Data is in
  `<DATA_DIR>` (default `./data` next to the binary); delete it to remove the vault.

> Since the vault is only decryptable with the master key / passphrase, leftover
> data isn't readable without them — but delete the data directory if you want it
> fully gone.

## Disaster & recovery

First, the two secrets and how they differ:

- **Master key** — the random key that actually encrypts every stored secret. You
  never see or type it; it's generated on first run and, in personal mode, kept
  **sealed** in `<DATA_DIR>/master.key`. It never changes.
- **Passphrase** — does *not* encrypt your data. It only *unseals* the master key
  (Argon2id-derived wrapping). It's the one secret you set and remember. In server
  mode there's no passphrase — the raw key is supplied via `MASTER_KEY` instead.

Recovery scenarios:

| Situation | Recoverable? | How |
|---|---|---|
| Forgot an **admin login password** | ✅ Yes | `-reset-admin -email X -reset-password` (prompts for the vault passphrase, then a new password). Data untouched. |
| Lost an **authenticator / 2FA** | ✅ Yes | `-reset-admin -email X -clear-mfa`. |
| **Every** admin locked out | ✅ Yes | Unlock the vault, remove all admins, and the first-run register flow reopens (data stays intact — it's encrypted with the master key, not passwords). |
| Forgot the **vault passphrase** | ❌ **No** | The master key can't be unsealed, so all encrypted data is **permanently lost** — unless you separately kept the raw `MASTER_KEY`. |
| **Change** the passphrase | ✅ Yes | `-change-passphrase` (needs the current one; re-seals the same key, no data re-encryption). |

**The one thing you must not lose is the passphrase.** It's your single point of
failure — there is no backdoor and no reset for it. Store it somewhere safe
(offline, or a *different* password manager). If you'd rather hold a recoverable
copy of the key itself, run in `MASTER_KEY` mode and back that value up instead.

A **password reset requires the passphrase** on purpose: it proves you're the vault
owner, so a stolen DB file or backup can't be used to mint a working login.

## Building from source

Prebuilt binaries are on the Releases page; to build yourself, build the frontend
first (it's embedded into the binary via `go:embed` and is **not** committed):
```bash
cd web && npm ci && npm run build   # outputs internal/webui/dist/
cd .. && go build -o account-manager ./cmd/api
```
(`go install …` alone won't work — it skips the frontend build step.)

## Security notes

- Sessions use an **in-memory access token** + an **httpOnly refresh cookie**;
  active sessions renew silently, idle ones expire. `Secure` cookies + HSTS engage
  automatically over HTTPS.
- The **audit log** (Settings → Logs) records every reveal/change/export with the
  client IP — see [`AUDIT_LOG.md`](AUDIT_LOG.md). Server/console messages are in
  the same log; codes are explained there.
- **Login rate-limiting:** the app limits attempts **per email**. For a public
  deployment, add **per-IP / global** limits at your **reverse proxy** (nginx
  `limit_req`, Caddy `rate_limit`, or Cloudflare) — the edge sees the true client
  IP, so that's where IP-based limits belong.
- Full security posture: [`THREAT_MODEL.md`](THREAT_MODEL.md).
