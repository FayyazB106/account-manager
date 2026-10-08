Account Manager
===============

Launch it from the Start Menu ("Account Manager"). The server starts in the
background (no window), a tray icon appears near the clock, and your browser
opens automatically at:

    http://localhost:8080

(If you change the port in Settings -> Deployment, the tray icon opens the new
port after you restart.)

FIRST RUN
  Everything happens in the browser: you'll choose a passphrase that protects the
  master key (which encrypts every stored secret), then create the first admin
  account. Keep this passphrase safe — without it (and the master key) the
  encrypted data cannot be recovered. On later starts you'll just enter the
  passphrase to unlock.

DATA
  Your database, key file, and backups are stored in a "data" folder next to the
  installed program. Back it up.

STOP
  Right-click the tray icon (near the clock) and choose "Quit".

TROUBLESHOOTING
  If the browser can't reach the app, check "server.log" in the "data" folder next
  to the installed program for startup errors.

SERVER USE
  For a networked deployment (a domain, HTTPS), run the binary on a host with a
  MASTER_KEY set and put it behind your reverse proxy. See DEPLOY.md in the
  project repository for full instructions.
