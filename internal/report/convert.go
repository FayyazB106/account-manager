// Package report builds the payments spreadsheet shared by the main Excel export
// and the monthly email, and runs the scheduler that emails it.
package report

import (
	"math"

	"github.com/FayyazB106/account-manager/internal/store"
)

// Converter turns amounts between currencies using the rates in the currency
// table. It mirrors the frontend's money.ts (amount ÷ rate → USD, then × target
// rate) so the server's spreadsheets agree with the Finance page.
type Converter struct {
	rate   map[string]float64
	active []store.Currency
}

// NewConverter builds a Converter from the full currency list. "Active" currencies
// are those switched on with a usable rate (USD, the base, is always usable).
func NewConverter(all []store.Currency) *Converter {
	c := &Converter{rate: map[string]float64{}}
	for _, cur := range all {
		c.rate[cur.Code] = cur.Rate
		if cur.Active && (cur.Code == "USD" || cur.Rate > 0) {
			c.active = append(c.active, cur)
		}
	}
	return c
}

// Active returns the currencies that get an Amount column, in table order.
func (c *Converter) Active() []store.Currency { return c.active }

// ToUSD converts amount (expressed in code) to USD. With no usable rate the
// amount is returned unchanged so a total never silently drops a payment.
func (c *Converter) ToUSD(amount float64, code string) float64 {
	if code == "USD" {
		return amount
	}
	if r := c.rate[code]; r > 0 {
		return amount / r
	}
	return amount
}

// AmountIn converts amount (in code) into the target currency, rounded to 2dp.
func (c *Converter) AmountIn(amount float64, code string, target store.Currency) float64 {
	usd := c.ToUSD(amount, code)
	if target.Code == "USD" {
		return Round2(usd)
	}
	return Round2(usd * target.Rate)
}

// Round2 rounds to two decimal places.
func Round2(v float64) float64 { return math.Round(v*100) / 100 }
