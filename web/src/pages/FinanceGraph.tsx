import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { FaChartLine, FaChartColumn } from "react-icons/fa6";
import { FaFileExcel } from "react-icons/fa";
import { useLayout } from "../components/Layout";
import { withUsdAmounts } from "../money";
import { CircleButton } from "../components/CircleButton";
import { downloadSheet } from "./FinanceAccounts";
import { fmtMoney } from "../money";
import type { AccountListItem } from "../api";

interface MonthData {
  label: string;     // English axis label (e.g. "Oct", "Jan 2027")
  monthDate: Date;   // first of the month, for localized labels
  value: number; // USD total
  items: { id: string; service: string; login?: string; amount: number; date: string }[];
}

function daysInMonth(y: number, m: number): number { return new Date(y, m + 1, 0).getDate(); }

// Add `add` months to a date, clamping the day to the target month's length so a
// Jan-31 (or 29th) start doesn't skip a short month like February.
function addMonthsClamped(base: Date, add: number): Date {
  const d = new Date(base.getFullYear(), base.getMonth() + add, 1);
  d.setDate(Math.min(base.getDate(), daysInMonth(d.getFullYear(), d.getMonth())));
  return d;
}

// Project each account's recurring charge across the next 12 months, keeping the
// per-month list of which accounts pay.
function projectMonths(accounts: AccountListItem[]): MonthData[] {
  const now = new Date();
  const start = new Date(now.getFullYear(), now.getMonth(), 1);
  const months: MonthData[] = Array.from({ length: 12 }, (_, i) => {
    const d = new Date(start.getFullYear(), start.getMonth() + i, 1);
    // Axis shows the short month only; the year lives in the tooltip/detail.
    const short = d.toLocaleString("en", { month: "short" });
    return { label: short, monthDate: d, value: 0, items: [] };
  });
  const idxOf = (d: Date) => (d.getFullYear() - start.getFullYear()) * 12 + (d.getMonth() - start.getMonth());

  const isoDay = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;

  for (const a of accounts) {
    if (a.status !== "active") continue;
    if (!a.amount || a.amount <= 0 || !a.next_payment_date) continue;
    const period = a.billing_period_months;
    const startPay = new Date(a.next_payment_date);
    const push = (i: number, occ: Date) => {
      if (i >= 0 && i < 12) { months[i].value += a.amount!; months[i].items.push({ id: a.id, service: a.service, login: a.login, amount: a.amount!, date: isoDay(occ) }); }
    };
    if (period <= 0) { push(idxOf(startPay), startPay); continue; }
    for (let k = 0; k < 200; k++) {
      const occ = addMonthsClamped(startPay, k * period);
      const i = idxOf(occ);
      if (i >= 12) break;
      push(i, occ);
    }
  }
  return months;
}

export function FinanceGraph() {
  const { accounts: rawAccounts, currencies } = useLayout();
  // Each account has its own currency; the chart adds amounts up, so work in USD.
  const accounts = useMemo(() => withUsdAmounts(rawAccounts, currencies), [rawAccounts, currencies]);
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const [ccy, setCcy] = useState("USD");
  const [selected, setSelected] = useState<number | null>(null);
  const [chartType, setChartType] = useState<"line" | "bar">("line");

  const activeCcys = currencies.filter((c) => c.active && (c.code === "USD" || c.rate > 0));
  const fx = ccy === "USD" ? 1 : (currencies.find((c) => c.code === ccy)?.rate ?? 1);
  // The line chart is always rendered English/LTR (a left-to-right time series),
  // but the tooltip and detail header use a localized month label.
  const months = projectMonths(accounts);
  const locLabel = (d: Date) => d.toLocaleString(i18n.language, { month: "long", year: "numeric" });
  const chart = months.map((m) => ({ label: m.label, tip: locLabel(m.monthDate), value: m.value * fx }));

  const sel = selected !== null ? months[selected] : null;
  // Selected month, or (if none) every month with payments — same as the list.
  const detailBlocks = sel ? [sel] : months.filter((m) => m.items.length > 0);

  // Export what's shown: selected month or all 12, in the chosen currency.
  function exportExcel() {
    const money2 = (v: number) => Number((v * fx).toFixed(2));
    const header = [t("finance.month"), t("finance.service"), t("finance.login"), t("accounts.nextPaymentShort"), `${t("accounts.amount")} (${ccy})`];
    const data: (string | number)[][] = [];
    for (const m of detailBlocks) {
      for (const it of m.items) {
        data.push([locLabel(m.monthDate), it.service, it.login ?? "", it.date, money2(it.amount)]);
      }
    }
    downloadSheet([header, ...data], t("finance.monthlyCost"), `monthly-cost-${ccy}.xlsx`);
  }

  return (
    <div className="card">
      <div className="topbar" style={{ marginBottom: 12 }}>
        <div>
          <h2 className="section-title" style={{ margin: 0 }}>{t("finance.monthlyCost")}</h2>
          <p className="muted" style={{ margin: "2px 0 0" }}>{t("finance.monthlyCostSubtitle")}</p>
        </div>
        <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
          <div className="chart-toggle">
            <button className={chartType === "line" ? "active" : ""} title={t("finance.lineChart")} onClick={() => setChartType("line")}><FaChartLine /></button>
            <button className={chartType === "bar" ? "active" : ""} title={t("finance.barChart")} onClick={() => setChartType("bar")}><FaChartColumn /></button>
          </div>
          {activeCcys.length > 1 && (
            <select dir="ltr" value={ccy} onChange={(e) => setCcy(e.target.value)} title={t("finance.displayCurrency")}>
              {activeCcys.map((c) => <option key={c.code} value={c.code}>{c.code}</option>)}
            </select>
          )}
          <CircleButton variant="ok" title={t("export.exportBtn")} onClick={exportExcel}><FaFileExcel /></CircleButton>
        </div>
      </div>

      {chartType === "line"
        ? <LineChart data={chart} format={(v) => fmtMoney(ccy, v)} selected={selected} onSelect={setSelected} />
        : <BarChart data={chart} format={(v) => fmtMoney(ccy, v)} selected={selected} onSelect={setSelected} />}

      {/* A month is selected → show just it. Nothing selected → show every month
          with payments, stacked earliest-first. */}
      {(() => {
        const blocks = detailBlocks;
        if (blocks.length === 0) return null;
        return (
          <div className="month-detail">
            {blocks.map((m) => (
              <div key={m.monthDate.toISOString()} className="month-block">
                <strong className="month-detail-head">{locLabel(m.monthDate)} — <span dir="ltr">{fmtMoney(ccy, m.value * fx)}</span></strong>
                {m.items.length === 0 ? (
                  <p className="muted">{t("finance.noPayments")}</p>
                ) : (
                  <ul className="cat-list">
                    {m.items.map((it, i) => (
                      <li key={i} className="clickable-row" onClick={() => navigate(`/account/${it.id}`)}>
                        <span className="md-account">
                          <span className="md-service">{it.service}</span>
                          {it.login && <span className="muted md-login">{it.login}</span>}
                          <span className="muted md-date">{it.date}</span>
                        </span>
                        <span dir="ltr">{fmtMoney(ccy, it.amount * fx)}</span>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            ))}
          </div>
        );
      })()}
    </div>
  );
}

// Catmull-Rom → cubic Bézier for a smooth curve through the points.
function smoothPath(pts: [number, number][]): string {
  if (pts.length < 2) return "";
  let d = `M ${pts[0][0]},${pts[0][1]}`;
  for (let i = 0; i < pts.length - 1; i++) {
    const p0 = pts[i - 1] || pts[i];
    const p1 = pts[i];
    const p2 = pts[i + 1];
    const p3 = pts[i + 2] || p2;
    const cp1x = p1[0] + (p2[0] - p0[0]) / 6;
    const cp2x = p2[0] - (p3[0] - p1[0]) / 6;
    // Clamp control-point Y to the segment's own range so the curve never
    // overshoots past its endpoints (which was dipping the line below zero).
    const loY = Math.min(p1[1], p2[1]), hiY = Math.max(p1[1], p2[1]);
    const clamp = (v: number) => Math.max(loY, Math.min(hiY, v));
    const cp1y = clamp(p1[1] + (p2[1] - p0[1]) / 6);
    const cp2y = clamp(p2[1] - (p3[1] - p1[1]) / 6);
    d += ` C ${cp1x},${cp1y} ${cp2x},${cp2y} ${p2[0]},${p2[1]}`;
  }
  return d;
}

function LineChart({
  data, format, selected, onSelect, rtl,
}: {
  data: { label: string; tip?: string; value: number }[];
  format: (v: number) => string;
  selected: number | null;
  onSelect: (i: number | null) => void;
  rtl?: boolean;
}) {
  const W = 760, H = 280;
  const padL = 76, padR = 16, padT = 16, padB = 32;
  const plotW = W - padL - padR;
  const plotH = H - padT - padB;
  const [hover, setHover] = useState<number | null>(null);

  const niceMax = niceCeil(Math.max(1, ...data.map((d) => d.value)));
  // Value axis stays on the left in all languages. In RTL the months run
  // right-to-left so the series still reads chronologically.
  const x = (i: number) => {
    const j = rtl ? data.length - 1 - i : i;
    return padL + (j * plotW) / (data.length - 1);
  };
  const y = (v: number) => padT + plotH - (v / niceMax) * plotH;
  const pts = data.map((d, i) => [x(i), y(d.value)] as [number, number]);
  const gridLines = 4;

  return (
    <svg viewBox={`0 0 ${W} ${H}`} width="100%" role="img" className="linechart" style={{ direction: "ltr" }}
      onClick={() => onSelect(null)}>
      {Array.from({ length: gridLines + 1 }, (_, g) => {
        const val = (niceMax / gridLines) * g;
        const yy = y(val);
        return (
          <g key={g}>
            <line x1={padL} y1={yy} x2={W - padR} y2={yy} className="lc-grid" />
            <text x={padL - 8} y={yy + 4} textAnchor="end" className="lc-axis">{format(val)}</text>
          </g>
        );
      })}

      <path d={smoothPath(pts)} className="lc-line" fill="none" pathLength={1} />

      {data.map((d, i) => {
        // Anchor the two edge labels inward so wide labels don't clip off the
        // chart edge; middle labels stay centered.
        const visual = rtl ? data.length - 1 - i : i;
        const anchor = visual === 0 ? "start" : visual === data.length - 1 ? "end" : "middle";
        return (
        <g key={i}>
          {/* wide invisible hit target for easy clicking/hovering */}
          <circle
            cx={x(i)} cy={y(d.value)} r={14} fill="transparent" style={{ cursor: "pointer" }}
            onClick={(e) => { e.stopPropagation(); onSelect(i); }}
            onMouseEnter={() => setHover(i)}
            onMouseLeave={() => setHover((h) => (h === i ? null : h))}
          />
          <circle cx={x(i)} cy={y(d.value)} r={selected === i ? 6 : 4} className={`lc-dot ${selected === i ? "sel" : ""} ${hover === i ? "hov" : ""}`} style={{ pointerEvents: "none" }} />
          <text x={x(i)} y={H - 10} textAnchor={anchor} className="lc-axis">{d.label}</text>
        </g>
        );
      })}

      {hover !== null && <Tooltip x={x(hover)} y={y(data[hover].value)} label={data[hover].tip ?? data[hover].label} value={format(data[hover].value)} chartW={W} />}
    </svg>
  );
}

function BarChart({
  data, format, selected, onSelect,
}: {
  data: { label: string; tip?: string; value: number }[];
  format: (v: number) => string;
  selected: number | null;
  onSelect: (i: number | null) => void;
}) {
  const W = 760, H = 280;
  const padL = 76, padR = 16, padT = 16, padB = 32;
  const plotW = W - padL - padR;
  const plotH = H - padT - padB;
  const [hover, setHover] = useState<number | null>(null);

  const niceMax = niceCeil(Math.max(1, ...data.map((d) => d.value)));
  const n = data.length;
  const slotW = plotW / n;
  const barW = Math.min(slotW * 0.6, 40);
  const cx = (i: number) => padL + slotW * i + slotW / 2;
  const y = (v: number) => padT + plotH - (v / niceMax) * plotH;
  const baseY = padT + plotH;
  const gridLines = 4;

  return (
    <svg viewBox={`0 0 ${W} ${H}`} width="100%" role="img" className="linechart" style={{ direction: "ltr" }}
      onClick={() => onSelect(null)}>
      {Array.from({ length: gridLines + 1 }, (_, g) => {
        const val = (niceMax / gridLines) * g;
        const yy = y(val);
        return (
          <g key={g}>
            <line x1={padL} y1={yy} x2={W - padR} y2={yy} className="lc-grid" />
            <text x={padL - 8} y={yy + 4} textAnchor="end" className="lc-axis">{format(val)}</text>
          </g>
        );
      })}

      {data.map((d, i) => {
        const anchor = i === 0 ? "start" : i === n - 1 ? "end" : "middle";
        const barH = baseY - y(d.value);
        return (
          <g key={i}
            style={{ cursor: "pointer" }}
            onClick={(e) => { e.stopPropagation(); onSelect(i); }}
            onMouseEnter={() => setHover(i)}
            onMouseLeave={() => setHover((h) => (h === i ? null : h))}
          >
            {/* hit target: the bar's column only (not the empty space above), so
                clicking above a bar deselects via the svg background */}
            <rect x={cx(i) - slotW / 2} y={y(d.value) - 6} width={slotW} height={baseY - y(d.value) + 6} fill="transparent" />
            <rect
              x={cx(i) - barW / 2} y={y(d.value)} width={barW} height={Math.max(barH, 0)} rx={4}
              className={`bc-bar ${selected === i ? "sel" : ""} ${hover === i ? "hov" : ""}`}
              style={{ animationDelay: `${i * 35}ms` }}
            />
            <text x={cx(i)} y={H - 10} textAnchor={anchor} className="lc-axis">{d.label}</text>
          </g>
        );
      })}

      {hover !== null && <Tooltip x={cx(hover)} y={y(data[hover].value)} label={data[hover].tip ?? data[hover].label} value={format(data[hover].value)} chartW={W} />}
    </svg>
  );
}

// A styled hover card drawn in SVG space (a rounded rect + two text lines),
// positioned above the point and clamped to stay inside the chart width.
function Tooltip({ x, y, label, value, chartW }: { x: number; y: number; label: string; value: string; chartW: number }) {
  const w = 118, h = 46, gap = 14;
  const cx = Math.max(w / 2 + 4, Math.min(chartW - w / 2 - 4, x));
  let top = y - gap - h;
  const below = top < 0;
  if (below) top = y + gap;
  return (
    <g className="lc-tip" style={{ pointerEvents: "none" }}>
      <rect x={cx - w / 2} y={top} width={w} height={h} rx={8} className="lc-tip-box" />
      <text x={cx} y={top + 18} textAnchor="middle" className="lc-tip-label">{label}</text>
      <text x={cx} y={top + 36} textAnchor="middle" className="lc-tip-value">{value}</text>
    </g>
  );
}

function niceCeil(n: number): number {
  if (n <= 0) return 1;
  const pow = Math.pow(10, Math.floor(Math.log10(n)));
  const step = pow / 2;
  return Math.ceil(n / step) * step;
}
