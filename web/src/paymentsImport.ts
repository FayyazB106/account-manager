import * as XLSX from "xlsx";

export interface PaymentRow {
  paid_on: string; // YYYY-MM-DD
  amount: number;
  currency?: string; // empty = the account's own currency (the server fills it in)
  note?: string;
}
export type RowErrorReason = "date" | "amount" | "currency";
export interface RowError { row: number; reason: RowErrorReason }

// A real Excel date is a serial number (days since 1900). Only numbers in a sensible range
// count, so a plain amount typed in the date column is not mistaken for a date.
function toIsoDate(v: unknown): string | null {
  const pad = (n: number) => String(n).padStart(2, "0");
  let y = 0, m = 0, d = 0;
  if (typeof v === "number") {
    if (v < 20000 || v > 80000) return null; // roughly 1954 to 2119
    // Excel counts days from 1899-12-30 (25569 days before the Unix epoch); drop any time of day.
    const dt = new Date(Math.floor(v - 25569) * 86400000);
    y = dt.getUTCFullYear(); m = dt.getUTCMonth() + 1; d = dt.getUTCDate();
  } else if (typeof v === "string") {
    // Only year-first dates: day/month order is ambiguous, so it is never guessed.
    const hit = v.trim().match(/^(\d{4})[-/.](\d{1,2})[-/.](\d{1,2})$/);
    if (!hit) return null;
    y = Number(hit[1]); m = Number(hit[2]); d = Number(hit[3]);
  } else {
    return null;
  }
  const check = new Date(Date.UTC(y, m - 1, d));
  if (check.getUTCFullYear() !== y || check.getUTCMonth() !== m - 1 || check.getUTCDate() !== d) return null;
  return `${y}-${pad(m)}-${pad(d)}`;
}

function toAmount(v: unknown): number | null {
  if (typeof v === "number") return Number.isFinite(v) && v >= 0 ? v : null;
  if (typeof v === "string") {
    const n = Number(v.trim().replace(/[\s,]/g, ""));
    return v.trim() !== "" && Number.isFinite(n) && n >= 0 ? n : null;
  }
  return null;
}

// readPaymentsFile reads a spreadsheet (.xlsx, .xls) or CSV of payments from its first sheet:
// A = date, B = amount, C = currency (optional), D = note (optional). No header row. Rows that
// can't be read are returned in `errors` (with their row number) and left out of `rows`;
// completely empty rows are ignored.
export async function readPaymentsFile(file: File): Promise<{ rows: PaymentRow[]; errors: RowError[] }> {
  // A CSV is read as UTF-8 text (see peopleImport.ts for why).
  const isCsv = /\.csv$/i.test(file.name) || file.type === "text/csv";
  const wb = isCsv ? XLSX.read(await file.text(), { type: "string" }) : XLSX.read(await file.arrayBuffer());
  const sheet = wb.Sheets[wb.SheetNames[0]];
  const first = sheet["!ref"] ? XLSX.utils.decode_range(sheet["!ref"]).s.r : 0;
  const data = XLSX.utils.sheet_to_json<unknown[]>(sheet, { header: 1 });

  const rows: PaymentRow[] = [];
  const errors: RowError[] = [];
  data.forEach((r, i) => {
    if (!Array.isArray(r) || r.every((c) => c === undefined || c === null || String(c).trim() === "")) return;
    const row = first + i + 1;
    const paid_on = toIsoDate(r[0]);
    if (!paid_on) { errors.push({ row, reason: "date" }); return; }
    const amount = toAmount(r[1]);
    if (amount === null) { errors.push({ row, reason: "amount" }); return; }
    const cur = String(r[2] ?? "").trim().toUpperCase();
    if (cur !== "" && !/^[A-Z]{3}$/.test(cur)) { errors.push({ row, reason: "currency" }); return; }
    const note = String(r[3] ?? "").trim();
    rows.push({ paid_on, amount, currency: cur || undefined, note: note || undefined });
  });
  return { rows, errors };
}
