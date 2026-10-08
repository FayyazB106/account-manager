package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/FayyazB106/account-manager/internal/api"
	"github.com/FayyazB106/account-manager/internal/backup"
	"github.com/FayyazB106/account-manager/internal/breach"
	"github.com/FayyazB106/account-manager/internal/config"
	"github.com/FayyazB106/account-manager/internal/db"
	"github.com/FayyazB106/account-manager/internal/rates"
	"github.com/FayyazB106/account-manager/internal/store"
	"github.com/FayyazB106/account-manager/internal/update"
	"github.com/FayyazB106/account-manager/internal/webui"
)

// version is the build version, injected via -ldflags "-X main.version=<tag>".
// "dev" for local/unstamped builds (which never report an available update).
var version = "dev"

func main() {
	sealKey := flag.Bool("seal-master-key", false, "seal the current MASTER_KEY into a passphrase-protected file, then exit")
	changePass := flag.Bool("change-passphrase", false, "change the passphrase protecting the sealed master key, then exit")
	genKey := flag.Bool("gen-key", false, "print a new random base64 MASTER_KEY (for env-based unattended deployments), then exit")
	resetAdmin := flag.Bool("reset-admin", false, "recover an admin: reset password and/or clear 2FA, then exit")
	resetEmail := flag.String("email", "", "admin email for -reset-admin (omit to list admins)")
	resetPw := flag.Bool("reset-password", false, "with -reset-admin: set a new password (prompted)")
	clearMFA := flag.Bool("clear-mfa", false, "with -reset-admin: disable and clear two-factor")
	tray := flag.Bool("tray", false, "run with a system-tray indicator and open the browser (desktop use)")
	flag.Parse()

	if *genKey {
		if err := genKeyCmd(); err != nil {
			log.Fatalf("gen key: %v", err)
		}
		return
	}

	// 1. Load configuration (data dir, key/JWT paths, port).
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Desktop mode (-tray): the launcher runs us hidden with no console, so send our
	// logs to a file ourselves. Go opens files with shared access, so a second
	// instance can append without the lock conflict a shell "> server.log" redirect
	// would cause (that lock is what made the shortcut do nothing while running).
	if *tray {
		if f, ferr := os.OpenFile(filepath.Join(cfg.DataDir, "server.log"),
			os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); ferr == nil {
			log.SetOutput(f)
		}
		// Single-instance: if a server is already running (the shortcut was clicked
		// again), just open it in the browser and exit instead of starting a second
		// instance that would fail to bind the port.
		if url := runningInstanceURL(cfg); url != "" {
			openBrowser(url)
			return
		}
	}

	// One-time maintenance: move an env MASTER_KEY into a sealed key file.
	if *sealKey {
		if err := sealMasterKeyCmd(cfg); err != nil {
			log.Fatalf("seal master key: %v", err)
		}
		return
	}
	// Change the passphrase protecting the sealed key.
	if *changePass {
		if err := changePassphraseCmd(cfg); err != nil {
			log.Fatalf("change passphrase: %v", err)
		}
		return
	}
	// Recover a locked-out admin (reset password / clear 2FA).
	if *resetAdmin {
		if err := resetAdminCmd(cfg, *resetEmail, *resetPw, *clearMFA); err != nil {
			log.Fatalf("reset admin: %v", err)
		}
		return
	}

	// 2. Resolve the master key without any console prompt. May be nil — then the
	//    app starts locked/uninitialized and the browser handles Setup/Unlock.
	masterKey, err := resolveStartupKey(cfg)
	if err != nil {
		log.Fatalf("master key: %v", err)
	}

	// JWT signing secret (env, or a persistent one under the data dir).
	jwtSecret, err := resolveJWTSecret(cfg)
	if err != nil {
		log.Fatalf("jwt secret: %v", err)
	}

	// 3. Open the embedded SQLite database (created + migrated on first run).
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer database.Close()
	log.Printf("database ready at %s", cfg.DBPath)

	settings := store.NewSettingStore(database)
	accounts := store.NewAccountStore(database)

	// Backup manager (scheduled, retained full-DB snapshots incl. attachments).
	backups, err := backup.NewManager(cfg.BackupDir, database, settings)
	if err != nil {
		log.Fatalf("backup dir: %v", err)
	}

	// 4. Build the API server with its dependencies.
	srv := &api.Server{
		Admins:      store.NewAdminStore(database),
		Accounts:    accounts,
		Attachments: store.NewAttachmentStore(database),
		Payments:    store.NewPaymentStore(database),
		Categories:  store.NewCategoryStore(database),
		People:      store.NewPersonStore(database),
		Settings:    settings,
		Currencies:  store.NewCurrencyStore(database),
		Audit:       store.NewAuditStore(database),
		Refresh:     store.NewRefreshStore(database),
		Backups:     backups,
		JWTSecret:   jwtSecret,
		KeyPath:     cfg.KeyPath,
		MasterKey:   masterKey, // may be nil until Setup/Unlock
		Version:     version,
		Desktop:     *tray, // only an interactive desktop session may apply updates
		Updater:     update.New(),
	}

	// Breach monitor reads the master key lazily (it may be set later via web
	// Unlock), so give it a live accessor rather than a captured value.
	breaches := breach.NewMonitor(accounts, settings, srv.MasterKeyValue)
	srv.Breach = breaches

	// System events (backups, breach scans, rate updates, startup) are recorded
	// in the in-app log as the "system" actor, alongside user actions.
	sysAudit := srv.Audit.SysLogger()
	backups.SetAuditLogger(sysAudit)
	breaches.SetAuditLogger(sysAudit)

	// Refresh exchange rates now and every 24h (public data — safe server-side).
	rates.StartDaily(srv.Currencies, sysAudit)

	// Run scheduled backups per the stored schedule.
	backups.StartScheduler()

	// Run scheduled breach monitoring per the stored cadence.
	breaches.StartScheduler()

	// Prune the login rate-limiter's in-memory map periodically.
	srv.StartRateLimiterSweeper()

	// Periodically prune expired/spent refresh tokens.
	go func() {
		t := time.NewTicker(1 * time.Hour)
		defer t.Stop()
		for {
			if err := srv.Refresh.Prune(context.Background()); err != nil {
				log.Printf("refresh prune: %v", err)
			}
			<-t.C
		}
	}()

	// Single-binary serving: the API lives under /api, and the embedded React SPA
	// is the catch-all for everything else. Mounting the API under a prefix avoids
	// path collisions (both use /accounts, /settings, /login, ...).
	root := http.NewServeMux()
	root.Handle("/api/", http.StripPrefix("/api", srv.Routes()))
	root.Handle("/", webui.Handler())

	addr := bindAddr(settings)
	handler := securityHeaders(root)

	tls := cfg.TLSCert != "" && cfg.TLSKey != ""
	scheme := "http"
	if tls {
		scheme = "https"
	}
	url := scheme + "://127.0.0.1:" + deployPort(settings)

	// serve blocks, listening on addr. TLS is served directly when a cert+key are
	// provided (TLS_CERT/TLS_KEY) — for direct-to-internet or LAN-with-cert setups.
	// The recommended networked path stays a reverse proxy terminating TLS (real
	// auto-renewing cert), where the app serves plain HTTP behind it. Localhost/
	// personal mode stays HTTP (a browser secure context already).
	serve := func() {
		// Bind explicitly so a port already in use (another running instance) is
		// caught here rather than silently failing. In desktop mode we then just
		// open the existing instance in the browser instead of dying.
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			if *tray {
				log.Printf("already running (%s in use); opening browser", addr)
				openBrowser(url)
				os.Exit(0)
			}
			log.Fatalf("listen %s: %v", addr, err)
		}
		writeEndpoint(cfg, url)
		defer removeEndpoint(cfg)

		if tls {
			log.Printf("account-manager listening on https://%s (UI + API, TLS)", addr)
			sysAudit("system.start", addr, "server started (TLS)", "ok")
			if err := http.ServeTLS(ln, handler, cfg.TLSCert, cfg.TLSKey); err != nil {
				log.Fatalf("server failed: %v", err)
			}
			return
		}
		log.Printf("account-manager listening on %s (UI + API)", addr)
		sysAudit("system.start", addr, "server started", "ok")
		if err := http.Serve(ln, handler); err != nil {
			log.Fatalf("server failed: %v", err)
		}
	}

	// Desktop mode: serve in the background and show a tray indicator that opens
	// the browser at the actual listening port (so a changed port is honoured).
	if *tray {
		go serve()
		runTray(url)
		return
	}

	serve()
}

// securityHeaders adds hardening headers to every response. The CSP is
// same-origin only (the SPA and API share the origin; the sole exceptions are
// inline styles from React/dialogs and data: URIs for the org logo and QR codes).
func securityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; " +
		"script-src 'self'; " +
		"style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; " +
		"font-src 'self'; " +
		// The Vault Health scan checks passwords against Have I Been Pwned from the
		// browser (k-anonymity — only a SHA-1 prefix is sent).
		"connect-src 'self' https://api.pwnedpasswords.com; " +
		"object-src 'none'; " +
		"base-uri 'self'; " +
		"frame-ancestors 'none'; " +
		"form-action 'self'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", csp)
		// HSTS only over HTTPS — direct TLS (r.TLS) or behind a TLS-terminating
		// proxy (X-Forwarded-Proto). Never sent on plain-HTTP/localhost.
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// bindAddr computes the listen address from the Deployment settings (set in
// Settings → Deployment), with environment overrides for the initial server
// bootstrap (before the UI is reachable to flip the toggle).
//
// Precedence: HOST/PORT env vars > stored deploy settings > defaults. The
// default is localhost:8080 (personal mode). Turning on "accessible on the
// network" binds 0.0.0.0 so the team can reach it via the mapped domain.
// Changes to the stored settings apply on the next restart.
func bindAddr(settings *store.SettingStore) string {
	host := "127.0.0.1"
	if all, err := settings.All(context.Background()); err == nil {
		if all["deploy_network"] == "on" {
			host = "0.0.0.0"
		}
	}
	if h := strings.TrimSpace(os.Getenv("HOST")); h != "" {
		host = h
	}
	return host + ":" + deployPort(settings)
}

// endpointPath is a small file the running instance writes with its URL, used for
// single-instance detection (see runningInstanceURL).
func endpointPath(cfg *config.Config) string {
	return filepath.Join(cfg.DataDir, "endpoint")
}

func writeEndpoint(cfg *config.Config, url string) {
	_ = os.WriteFile(endpointPath(cfg), []byte(url), 0o600)
}

func removeEndpoint(cfg *config.Config) {
	_ = os.Remove(endpointPath(cfg))
}

// runningInstanceURL returns the URL of an already-running instance, or "" if
// none. It reads the endpoint file and confirms something is actually listening
// there (so a stale file from a crash doesn't block a fresh start).
func runningInstanceURL(cfg *config.Config) string {
	b, err := os.ReadFile(endpointPath(cfg))
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(string(b))
	u, err := neturl.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	conn, err := net.DialTimeout("tcp", u.Host, 500*time.Millisecond)
	if err != nil {
		return "" // stale endpoint file; nothing listening
	}
	conn.Close()
	return raw
}

// deployPort resolves the listening port: PORT env override > stored deploy_port
// setting > default 8080. Shared by bindAddr and the tray's browser URL so both
// always agree on the port.
func deployPort(settings *store.SettingStore) string {
	port := "8080"
	if all, err := settings.All(context.Background()); err == nil {
		if p := strings.TrimSpace(all["deploy_port"]); p != "" {
			if n, perr := strconv.Atoi(p); perr == nil && n > 0 && n < 65536 {
				port = p
			}
		}
	}
	if p := strings.TrimSpace(os.Getenv("PORT")); p != "" {
		port = p
	}
	return port
}
