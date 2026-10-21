# Threat Model — Account Manager

A deliberately simple threat model, written to be learned from. As the project
grows, revisit each row.

## Current security posture (important — read first)

The intended model is **server-side envelope encryption**, not a client-side
zero-knowledge system. (A client-side "inner lock" was considered but is **not
currently planned** — see the note at the end.) Concretely:

- Secrets and attachments are encrypted with **server-side envelope encryption**:
  a per-secret Data Encryption Key (DEK) wrapped by a **`MASTER_KEY`** that lives
  **outside the database** (env var, or a passphrase-sealed key file — see the
  key-management notes in `README.md`), using AES-256-GCM.
- Because the server holds the `MASTER_KEY`, **the server can decrypt** — that is
  how reveal, Excel export, attachment download, and breach monitoring work.
- Therefore this is **encryption-at-rest against a DB-only leak**. A full server
  compromise (code execution + access to the `MASTER_KEY`) **would** expose
  plaintext — an accepted limitation for a trusted-team internal tool.

The rows below describe **what is true today**.

## What we are protecting

- **Primary asset:** the stored account passwords and invoice attachments.
- **Secondary:** who-uses-what metadata, subscription/billing info, org branding.
- The **master passwords** of the admins (never stored; only Argon2id derivatives).

## Who we defend against (and how)

| # | Threat | Scenario | Our defense (today) |
|---|---|---|---|
| T1 | **Database leak** | Attacker dumps only the DB (stolen backup, SQL injection, file theft). | Secrets & attachments are AES-256-GCM ciphertext; the wrapping `MASTER_KEY` is **not in the DB**. A DB-only dump is useless ciphertext. Logins are Argon2id hashes. |
| T2 | **Server compromise** | Attacker gets code execution on the host **and** the `MASTER_KEY`. | **Accepted limitation.** The server can decrypt, so this exposes plaintext. Mitigations: keep `MASTER_KEY` out of the repo/DB (env or passphrase-sealed file), least-privilege host. Full protection would need a client-side inner lock (not planned). |
| T3 | **Network eavesdropping** | Attacker on the wire between client and server. | **No TLS needed on `localhost`** (secure context). For a networked/org deployment, a **reverse proxy terminates TLS** on the domain (see `README.md`). |
| T4 | **Brute-force login** | Attacker guesses an admin password against the API. | Argon2id (slow by design) **+ per-email login rate-limiting with lockout** (5 fails → 5-min lockout) + anti-enumeration. **Optional TOTP MFA** (per admin) adds a second factor (also rate-limited). |
| T5 | **Weak / reused / breached stored passwords** | Accounts use guessable, shared, or already-leaked passwords. | CSPRNG **password generator** + **strength meter** + **Vault Health** audit (weak/reused/old), and **continuous breach monitoring** — a scheduled server-side Have I Been Pwned check (k-anonymity) that flags breached passwords without a manual scan. |
| T6 | **Lost/stolen device** | An admin's laptop or phone is taken. | Password (and optional TOTP) required to log in; **in-memory access token** (15 min, **never** in web storage) + an **httpOnly `SameSite=Strict` refresh cookie** the page's JS can't read; a reload drops the access token and re-derives it from the cookie. Sessions are **server-side revocable** (logout / password change). **Clipboard auto-clear** wipes copied secrets after a delay. Note: downloaded attachments are plaintext on disk (see below). |
| T7 | **Malicious/removed admin** | A former admin should lose access. | Delete the admin (removes login), guarded so the **last admin can't be deleted**. All actions are attributable via the **audit log**. (With a single server `MASTER_KEY` there is no per-admin key to rotate.) |
| T8 | **Malicious update / supply-chain** | An attacker ships a tampered release, or MITMs the download, to run malicious code on the client. | The update source repo is **pinned at build time** (a compile-time constant, never a DB/admin setting) so no one can redirect the download-and-execute updater at an untrusted release. Updates are fetched over **HTTPS from GitHub** and the download is **SHA256-verified against the release's `SHA256SUMS`** before it is ever executed; a mismatch is rejected. Update checks are **off-switchable**, and servers are **notify-only** (no unattended self-replace). **Residual risk:** the checksum file ships in the same release, so a fully compromised GitHub release/account could forge both — closing that needs **signed releases** (planned: minisign with a key baked into the app). |

## Other exposures we accept / track

- **Decrypted downloads land on disk.** Exporting or downloading an attachment
  writes an **unencrypted** copy outside the vault. Served with
  `Content-Disposition: attachment` + `X-Content-Type-Options: nosniff` so the
  browser never renders/executes it, but the file itself is then the user's to
  protect. Inline in-browser viewing is intentionally **not** offered.
- **Clipboard auto-clear can't reach OS clipboard history.** Copying a secret and
  auto-clearing wipes the *active* clipboard (so Ctrl+V no longer pastes it), but
  **Windows Clipboard History (Win+V) and cloud clipboard sync keep their own copy**
  that a web app cannot delete — browsers can only write the current clipboard, not
  purge history, and can't set the native "exclude from clipboard history" flags.
  Mitigation is at the OS level (disable/clear clipboard history).
- **Outbound network calls.** By default two, both without keys or personal data: a
  daily USD exchange-rate fetch (`open.er-api.com`), and — when breach monitoring is
  enabled — the Have I Been Pwned range API (`api.pwnedpasswords.com`) using
  **k-anonymity**: only the first 5 chars of a password's SHA-1 are sent, so the
  password never leaves the server and the API can't tell which one was checked.
  A third exists **only if an admin configures email reports** (next bullet).
- **Email reports and reminders move account data out of the vault.** When SMTP is
  configured and an admin opts in, the server emails them: last month's payment
  spreadsheet and the month's upcoming payments (both on the 1st), and, daily, reminders
  listing the payments due within 7 days or overdue and the passwords due for rotation, plus
  an immediate alert when a breach scan finds a newly breached password (service, login and breach count — never the password).
  Together these contain **payment dates, amounts, service names, account logins
  (usernames/emails), seat users, notes and invoice file names — never passwords, 2FA
  secrets or the invoice files themselves** — and once sent it sits **unencrypted in the recipient's mailbox and the provider's
  systems**, outside our control. Mitigations: it is **off by default** (a master switch in
  Settings → Email, plus a **per-admin opt-in**), sent only to that admin's own login email, and revocable by switching it
  off. The SMTP password is stored **envelope-encrypted with `MASTER_KEY`**, never
  returned by any API (not even to admins), and cannot be written through the generic
  settings endpoint; the mailer **refuses to send credentials over an unencrypted
  connection** and rejects header injection. A restore onto a different master key
  leaves the stored SMTP password unreadable until it is re-entered. The email's
  transport security is only as good as the SMTP provider's. Each admin's UI language (`en`/`ar`)
  is stored on the server so their emails can be written in it; it is a display preference, not
  sensitive.
  Any admin who can edit **Settings → Email** chooses the SMTP host and port the server connects
  to, so they can point it at any address the server can reach, including internal ones. This is
  a limited server-side request risk (the server only speaks SMTP, but the connection error
  shown when a send fails can reveal whether a host and port are reachable), accepted because admins are trusted and a mail relay on your own network
  is a legitimate setup. Don't give admin access to people you wouldn't trust with network access
  from the server.
- **The server decrypts passwords during breach monitoring.** With monitoring on,
  the scheduled job decrypts each stored password (with `MASTER_KEY`) to hash it.
  This is inherent to the server-side model (the server can already decrypt) and
  the plaintext stays in memory only for the check.
- **Stored 2FA secrets and password history** are encrypted the same way as
  passwords (`MASTER_KEY` envelope), so T1 covers them.
- **Spreadsheet import parses an uploaded file.** Bulk-importing people (Settings
  → Users) parses an `.xlsx`/`.csv` **in the browser** with SheetJS. That library's
  npm build carries a known prototype-pollution / ReDoS advisory with **no fix on
  the npm registry** (the patched build lives only on SheetJS's own CDN). We accept
  it: the parse is client-side, reachable only by an **already-authenticated,
  trusted admin** importing their own file, and every other spreadsheet path only
  *writes* (exports), which is unaffected. **Import only spreadsheets you trust.**
  Upgrading to the CDN build (or moving the parse server-side) would close it.

## Explicitly out of scope (for now)

- **The client-side zero-knowledge lock** — **not planned**; trust in the server
  is required (see the note at the end).
- **A malicious admin while still authorized.** All admins are trusted and can
  see everything by design.
- **Compromise of an admin's own machine while unlocked** (keylogger, malware).
- **Sophisticated supply-chain attacks** on dependencies (minimized by small
  dependency trees, not fully defended).
- **Physical coercion** of an admin ("rubber-hose").

## Key rules we follow

1. The `MASTER_KEY` is never committed and never in the DB — it's supplied via an
   env var **or** a passphrase-sealed key file outside the DB. `JWT_SECRET` is an
   env var or an auto-generated file under the data dir. (See `README.md`.)
2. All keys, nonces, salts, and generated passwords come from a **CSPRNG**
   (`crypto/rand`).
3. **Authentication ≠ encryption:** the login hash and the encryption key are
   independent; the login flow never derives or sees the `MASTER_KEY`.
4. No secret is logged, put in a URL/query string, or returned in list endpoints
   (only via explicit reveal).

## Possible future direction: zero-knowledge inner lock (shared Vault Key)

*Not built, and **not currently planned** — recorded here as a design option. It
would close T2 (server compromise) by adding a second, independent encryption
layer **on the client**, so the server stores only ciphertext it cannot decrypt.
Adopting it is a large effort and would require a recovery-key scheme (otherwise a
forgotten admin password becomes permanent data loss); for a trusted-team internal
tool, the server-side envelope above was chosen instead.*

**Defense in depth — two independent locks.** Every secret gets encrypted twice;
defeating one layer yields nothing:

1. **Inner lock (client-side, zero-knowledge):** the client encrypts the secret
   with a key derived from the admin's master password *before* sending it. The
   server can never remove this lock.
2. **Outer lock (server-side, envelope):** what we have today — a per-secret DEK
   wrapped by the `MASTER_KEY` kept outside the DB.

To recover one plaintext an attacker would then need **all** of: the DB, the
server `MASTER_KEY`, **and** an admin's master password.

**Shared Vault Key model.** Because all admins share full access, we wrap one key
per admin rather than doing per-item public-key sharing:

1. One random **Vault Key** encrypts every secret (AES-256-GCM).
2. Each admin's login password is run through **Argon2id** to derive a personal
   **key-encryption-key (KEK)**.
3. The Vault Key is **wrapped (encrypted) once per admin** with that admin's KEK.
   The DB stores all secrets + one wrapped Vault Key per admin.
4. On login, the admin's password unwraps *their* copy of the Vault Key, letting
   them decrypt everything. The server never learns the KEK or the Vault Key.
5. **Add admin** = wrap the Vault Key with their KEK. **Remove admin** = delete
   their wrapped copy and **rotate** the Vault Key (this is the revocation T7
   currently can't do with a single `MASTER_KEY`).

**Crypto primitives (both layers):**
- **KDF:** Argon2id — for the login hash *and*, separately, for the client-side KEK.
- **Symmetric:** AES-256-GCM (authenticated).
- **Envelope:** per-secret DEK wrapped by a key kept out of the DB.
- **Randomness:** CSPRNG only (`crypto/rand`) — keys, nonces, salts, generated passwords.

**Recovery caveat:** once secrets are bound to password-derived keys, "all admins
lose their passwords" becomes **permanent data loss**. A recovery-key / emergency
access scheme must be designed *together with* this lock.
