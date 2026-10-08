// Package breach checks stored passwords against the Have I Been Pwned breached-
// password corpus, and runs that check on a schedule ("continuous monitoring").
//
// Privacy: it uses HIBP's k-anonymity range API — only the first 5 hex chars of
// the password's SHA-1 are ever sent; the password never leaves the server and
// the API can't tell which password was checked.
package breach

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const rangeAPI = "https://api.pwnedpasswords.com/range/"

// Check returns how many times the password appears in HIBP (0 = not found).
func Check(ctx context.Context, client *http.Client, password string) (int, error) {
	sum := sha1.Sum([]byte(password))
	full := strings.ToUpper(hex.EncodeToString(sum[:]))
	prefix, suffix := full[:5], full[5:]

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rangeAPI+prefix, nil)
	if err != nil {
		return 0, err
	}
	// Add-Padding hides the exact result-set size from a network observer.
	req.Header.Set("Add-Padding", "true")

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("hibp: unexpected status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		sfx, countStr, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(sfx), suffix) {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(countStr))
		if err != nil {
			return 0, nil
		}
		return n, nil
	}
	return 0, scanner.Err()
}
