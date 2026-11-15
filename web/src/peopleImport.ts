import * as XLSX from "xlsx";

export interface ImportRow {
  name: string;
  name_ar?: string;
}

// readPeopleFile reads a spreadsheet (.xlsx, .xls) or CSV of people: column A is the English
// name, column B the Arabic name (only read when Arabic support is on). Rows without a name
// are skipped. There is no header row: every row with a name becomes a person.
export async function readPeopleFile(file: File, withArabic: boolean): Promise<ImportRow[]> {
  // A CSV is text: decode it as UTF-8 ourselves. Handing the raw bytes to the spreadsheet
  // reader makes it guess an old Western encoding when there is no BOM, which garbles Arabic
  // names. (text() also drops a BOM, and the reader finds the delimiter, comma or semicolon.)
  const isCsv = /\.csv$/i.test(file.name) || file.type === "text/csv";
  const wb = isCsv ? XLSX.read(await file.text(), { type: "string" }) : XLSX.read(await file.arrayBuffer());
  const sheet = wb.Sheets[wb.SheetNames[0]];
  const rows = XLSX.utils.sheet_to_json<string[]>(sheet, { header: 1 });
  const out: ImportRow[] = [];
  for (const r of rows) {
    if (!Array.isArray(r)) continue;
    const name = String(r[0] ?? "").trim();
    if (!name) continue;
    const nameAr = withArabic ? String(r[1] ?? "").trim() : "";
    out.push({ name, name_ar: nameAr || undefined });
  }
  return out;
}
