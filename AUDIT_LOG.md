# Audit Log Reference

The log (Settings → Logs) is an append-only record of every security-relevant
action taken in the app. It is rendered as plain terminal text, oldest at the
top, newest at the bottom, and can be exported to a `.txt` file (the Export
button downloads every matching entry, not just the current page).

## Line format

Each line has six comma-separated fields, plus an optional seventh **detail**
field when the entry carries extra context (e.g. a login `via 2FA`, or a
scheduled scan's counts):

```
timestamp, actor, action, target, ip, result[, detail]
2026-09-24 20:34:47, you@example.com, account.reveal, /accounts/df49780a-.../reveal, 127.0.0.1, ok
2026-09-25 13:46:24, system, breach.scan, -, local, ok, scheduled / 15 checked / 0 skipped
```

| Column | Meaning |
| --- | --- |
| **timestamp** | Local server time, `YYYY-MM-DD HH:MM:SS`. |
| **actor** | Email of the signed-in admin who performed the action. For a failed login it is the email that was *attempted*. `system` means a background job (see [System & background jobs](#system--background-jobs)). A dash (`-`) means no actor was resolved. |
| **action** | Stable action code (see table below). |
| **target** | The API path acted on. For accounts/people/etc. the UUID in the path identifies the specific record. For a failed login the target is the attempted email. |
| **ip** | Client IP. Honors `X-Forwarded-For` (first hop) when behind a reverse proxy, otherwise the direct connection address. `127.0.0.1` = same machine (localhost). |
| **result** | `ok` if the request succeeded (HTTP < 400), `fail` otherwise (e.g. wrong password, denied action, validation error). |
| **detail** | *(optional)* Extra context when present — e.g. `via 2FA` on a login, or `scheduled / N checked / M skipped` on a background scan. Omitted entirely when empty. |

> Reading lists (e.g. loading the accounts table) is **not** logged — only
> mutations, secret reveals, exports, downloads, and logins. Revealing a secret
> *is* logged; browsing the list of accounts is not.

## Action codes

### Authentication
| Action | Meaning |
| --- | --- |
| `auth.login` | Admin signed in. `fail` = wrong credentials; `ok` with detail `via 2FA` = completed a TOTP-protected login. |
| `auth.mfa` | TOTP code verification step during login (`fail` = wrong/expired code). |

### Accounts (credentials)
| Action | Meaning |
| --- | --- |
| `account.create` | New account added. |
| `account.update` | Account edited (may include a password change). |
| `account.delete` | Account deleted. |
| `account.status` | Account activated / deactivated. |
| `account.extend` | Renewal/expiry date rolled forward one billing period (via "Mark as paid"). |
| `payment.add` | A payment was added to an account's history. |
| `payment.update` | A recorded payment was edited. |
| `payment.delete` | A recorded payment was deleted. |
| `account.reveal` | A stored password was revealed. |
| `account.history_reveal` | A previous password from history was revealed. |
| `account.export` | Accounts exported to a spreadsheet. |
| `account.totp_set` | A 2FA (TOTP) secret was added to an account. |
| `account.totp_clear` | An account's 2FA secret was removed. |
| `vault.scan` | Bulk reveal of all secrets for the Vault Health scan / Authenticator (one entry per scan, not per account). |

### Attachments
| Action | Meaning |
| --- | --- |
| `attachment.upload` | File attached to an account. |
| `attachment.download` | Attachment downloaded. |
| `attachment.export` | An account's attachments exported. |
| `attachment.delete` | Attachment deleted. |

### Admins & profile
| Action | Meaning |
| --- | --- |
| `admin.create` | New admin account created. |
| `admin.delete` | Admin account deleted. |
| `profile.update` | Signed-in admin edited their own profile. |
| `profile.password` | Signed-in admin changed their own password. |
| `mfa.setup` | Started 2FA enrollment (generated a secret/QR). |
| `mfa.enable` | 2FA turned on for the admin. |
| `mfa.disable` | 2FA turned off for the admin. |
| `profile.monthly_report` | The signed-in admin turned their monthly payment-history email on or off. |
| `profile.payment_reminders` | The signed-in admin turned their payment-reminder emails on or off. |
| `profile.payment_reminder_days` | The signed-in admin changed which days their payment reminder is sent on. |
| `profile.rotation_reminder_days` | The signed-in admin changed which days their password-rotation reminder is sent on. |
| `profile.rotation_reminders` | The signed-in admin turned their password-rotation reminder emails on or off. |
| `profile.breach_alerts` | The signed-in admin turned their breach-alert email on or off. |
| `profile.upcoming_payments` | The signed-in admin turned their monthly upcoming-payments email on or off. |

> The language the app is showing is sent with every request and noted as the language your emails
> are written in. That is deliberately **not** logged — it is a display preference.

### Categories
| Action | Meaning |
| --- | --- |
| `category.create` / `category.update` / `category.delete` | Category managed. |
| `category.reorder` | Category order changed. |

### People (seat directory)
| Action | Meaning |
| --- | --- |
| `person.create` / `person.update` / `person.delete` | Person record managed. |
| `person.import` | People imported from a spreadsheet. |
| `person.delete_bulk` | Multiple people deleted at once. |

### Settings & maintenance
| Action | Meaning |
| --- | --- |
| `settings.update` | A setting was changed (the setting key is in the target path, e.g. `/settings/breach_monitor`). |
| `settings.currency` | A currency's rate/state was changed. |
| `settings.purge_arabic` | Arabic translation data was purged. |
| `breach.scan` | A breach (HIBP) scan was run. |
| `backup.create` / `backup.restore` / `backup.download` / `backup.delete` | Backup managed. |
| `smtp.update` | The email (SMTP) configuration was saved. The password is never logged. |
| `smtp.toggle` | Email was switched on or off (the master switch in Settings → Email). |
| `smtp.test` | A test email was sent to the signed-in admin. `fail` = the provider rejected it or was unreachable. |
| `report.preview` | An admin emailed themselves last month's payment report on demand (**Send Now**). |
| `reminder.preview` | An admin emailed themselves the current payment reminder on demand (**Send Now**). |
| `breach_alert.preview` | An admin emailed themselves the passwords currently flagged as breached on demand (**Send Now**). |
| `rotation.preview` | An admin emailed themselves the current password-rotation reminder on demand (**Send Now**). |
| `upcoming.preview` | An admin emailed themselves this month's upcoming payments on demand (**Send Now**). |

### System & background jobs
These are recorded by the server itself, with actor `system` (they also print to
the server's terminal). They cover the scheduled jobs and startup.

| Action | Meaning |
| --- | --- |
| `system.start` | The server started. The target is the bind address (e.g. `127.0.0.1:8080`). |
| `system.update` | An admin started an in-app update. The target is the version being installed; the download is checksum-verified before it runs. |
| `rates.update` | Exchange rates were refreshed (`fail` = the rates provider was unreachable; the previous rates are kept). |
| `breach.scan` (actor `system`) | A **scheduled** breach scan ran. The detail is `scheduled / N checked / M skipped`. `result` is `fail` when nothing could be checked but accounts were skipped — usually the server itself can't reach HIBP. |
| `backup.create` (actor `system`) | A **scheduled** backup ran (`ok` with the file name, or `fail`). |
| `upcoming.send` (actor `system`) | The **monthly upcoming-payments** email was sent. The target is the recipient, the detail is `YYYY-MM / N payments` (`ok`) or the month plus the error (`fail`). A month with no payments sends nothing and is not logged. |
| `breach.alert` (actor `system`) | A **breach alert** was emailed right after a scan found newly breached passwords (not scheduled). The target is the recipient, the detail is `N new` (`ok`, with `(retried)` when it only went through on a later attempt) or the error plus `(will retry)` (`fail`). A failed alert is kept and retried about every 30 minutes; only the first failure is logged. |
| `rotation.send` (actor `system`) | A **scheduled password-rotation reminder** (on one of the admin's chosen days) was emailed. The target is the recipient, the detail is `YYYY-MM-DD / N due` (`ok`) or the date plus the error (`fail`). Retried every 30 minutes until it goes through; only the first failure is logged. |
| `reminder.send` (actor `system`) | A **scheduled payment reminder** (on one of the admin's chosen days) was emailed. The target is the recipient, the detail is `YYYY-MM-DD / N due` (`ok`) or the date plus the error (`fail`). Retried every 30 minutes until it goes through; only the first failure is logged. |
| `report.send` (actor `system`) | The **monthly payment report** was emailed. The target is the recipient, the detail is `YYYY-MM / N payments` (`ok`) or the month plus the error (`fail`). A failed send is retried every 30 minutes until it goes through; only the first failure is logged. |

> **Why a scan can show `0 checked, N skipped`:** the count only rises when an
> account's password is decrypted *and* its HIBP lookup succeeds. If the
> **server** can't reach HIBP, every account is skipped and 0 are checked — even
> though the **Vault Health** page may still show accounts as breached, because
> that page runs its own HIBP check from the **browser**, which has separate
> network access. A run with `0 checked, N skipped` therefore points at the
> server's outbound network, not at your data.

## Reading tips

- **Same timestamp, many `account.reveal` lines** usually means a Vault Health
  scan or an Authenticator load — look for an adjacent `vault.scan` line.
- **A burst of `fail` on `auth.login`** from one IP is worth investigating.
- **`ip` other than `127.0.0.1`** means the action came from another machine on
  the network (only possible when network access is enabled in Settings →
  Deployment).
