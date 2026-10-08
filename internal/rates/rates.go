// Package rates fetches public USD-base exchange rates and stores them.
// Source: open.er-api.com (free, no API key, ~daily updates).
package rates

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/FayyazB106/account-manager/internal/store"
)

const endpoint = "https://open.er-api.com/v6/latest/USD"

// blockedCodes are currency codes we never store, even if the API returns them.
var blockedCodes = map[string]bool{
	"ILS": true,
}

type apiResponse struct {
	Result string             `json:"result"`
	Rates  map[string]float64 `json:"rates"`
}

// Update fetches the latest rates and stores them for known currencies.
func Update(ctx context.Context, cs *store.CurrencyStore) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch rates: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("rates api status %d", resp.StatusCode)
	}

	var data apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("decode rates: %w", err)
	}
	if data.Result != "success" || len(data.Rates) == 0 {
		return fmt.Errorf("rates api returned no data")
	}
	for code := range blockedCodes {
		delete(data.Rates, code)
	}
	return cs.UpsertRates(ctx, data.Rates, nameFor)
}

// StartDaily runs Update once now (in the background) and then every 24h.
// audit may be nil; when set, each run records a system audit event.
func StartDaily(cs *store.CurrencyStore, audit store.AuditFunc) {
	go func() {
		run := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := Update(ctx, cs); err != nil {
				log.Printf("rates update failed: %v", err)
				if audit != nil {
					audit("rates.update", "", "", "fail")
				}
			} else {
				log.Println("exchange rates updated")
				if audit != nil {
					audit("rates.update", "", "", "ok")
				}
			}
		}
		run()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			run()
		}
	}()
}
