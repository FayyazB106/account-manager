import type { Currency } from "./api";

// Format a value in a given currency ("$" for USD, otherwise "CODE 0.00").
export function fmtMoney(code: string, value: number): string {
  return code === "USD" ? `$${value.toFixed(2)}` : `${code} ${value.toFixed(2)}`;
}

// Convert an amount expressed in `code` to USD, using the rate table (rate =
// units of `code` per 1 USD). Falls back to the original value when there's no
// positive rate to convert with (so a total never silently drops the amount).
export function toUSD(amount: number, code: string, currencies: Currency[]): number {
  if (code === "USD") return amount;
  const rate = currencies.find((c) => c.code === code)?.rate ?? 0;
  return rate > 0 ? amount / rate : amount;
}

// Show a USD amount plus its conversion in every active non-USD currency,
// e.g. "$20.00 / SAR 75.00".
export function amountVariants(usd: number, currencies: Currency[]): string {
  const parts = [`$${usd.toFixed(2)}`];
  for (const c of currencies) {
    if (c.active && c.code !== "USD" && c.rate > 0) parts.push(`${c.code} ${(usd * c.rate).toFixed(2)}`);
  }
  return parts.join(" / ");
}

// An account's amount is in its own currency (`currency`, USD when unset). Anything that
// ADDS amounts up or draws them on a chart needs USD figures first: pass the accounts
// through this and every amount becomes USD (currency "USD"), converted with the saved
// rates. Accounts without an amount are returned untouched.
export function withUsdAmounts<T extends { amount?: number | null; currency?: string }>(accounts: T[], currencies: Currency[]): T[] {
  return accounts.map((a) =>
    a.amount == null ? a : { ...a, amount: toUSD(a.amount, a.currency || "USD", currencies), currency: "USD" },
  );
}

// Show an amount in its own currency first, then its conversion into USD and every other
// active currency, e.g. "SAR 60.00 / $16.00". For a USD amount this is exactly
// amountVariants ("$16.00 / SAR 60.00").
export function amountVariantsFrom(amount: number, code: string, currencies: Currency[]): string {
  const usd = toUSD(amount, code, currencies);
  const parts = [fmtMoney(code, amount)];
  if (code !== "USD") parts.push(fmtMoney("USD", usd));
  for (const c of currencies) {
    if (c.active && c.code !== "USD" && c.code !== code && c.rate > 0) parts.push(`${c.code} ${(usd * c.rate).toFixed(2)}`);
  }
  return parts.join(" / ");
}
