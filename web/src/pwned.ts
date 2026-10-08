// pwned.ts — check a password against Have I Been Pwned's Pwned Passwords using
// k-anonymity, entirely from the browser.
//
// Privacy: we SHA-1 the password and send ONLY the first 5 hex chars of the hash
// to the API. It returns all breached-hash suffixes sharing that prefix; we match
// the rest of our hash LOCALLY. The password and its full hash never leave the
// device — consistent with the vault's zero-knowledge design. "Add-Padding" asks
// the API to pad the response so the real prefix can't be inferred from size.

async function sha1Hex(text: string): Promise<string> {
  const bytes = new TextEncoder().encode(text);
  const digest = await crypto.subtle.digest("SHA-1", bytes);
  return [...new Uint8Array(digest)]
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("")
    .toUpperCase();
}

// pwnedCount returns how many times the password appears in known breaches
// (0 = not found). Returns -1 if the check couldn't run (offline, blocked).
export async function pwnedCount(password: string): Promise<number> {
  try {
    const hash = await sha1Hex(password);
    const prefix = hash.slice(0, 5);
    const suffix = hash.slice(5);

    const res = await fetch(`https://api.pwnedpasswords.com/range/${prefix}`, {
      headers: { "Add-Padding": "true" },
    });
    if (!res.ok) return -1;

    const body = await res.text();
    for (const line of body.split("\n")) {
      const [suf, count] = line.trim().split(":");
      if (suf === suffix) return parseInt(count, 10) || 0;
    }
    return 0;
  } catch {
    return -1;
  }
}
