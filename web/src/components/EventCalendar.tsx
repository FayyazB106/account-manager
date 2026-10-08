import { useState } from "react";
import { useTranslation } from "react-i18next";
import { FaChevronLeft, FaChevronRight } from "react-icons/fa";
import { useMediaQuery } from "../hooks";
import type { AccountListItem } from "../api";

// EventCalendar shows the current month + next 2, marking payment and rotation
// dates. Hovering a day shows its events (title tooltip); clicking opens the
// first account for that day. On narrow screens it shows one month at a time
// with prev/next arrows instead of a wide multi-month grid.

interface Ev { type: "payment" | "rotation"; label: string; id: string; }

function toKey(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

function daysInMonth(y: number, m: number): number { return new Date(y, m + 1, 0).getDate(); }

// Add `add` months, clamping the day so e.g. Jan 31 doesn't skip February.
function addMonthsClamped(base: Date, add: number): Date {
  const d = new Date(base.getFullYear(), base.getMonth() + add, 1);
  d.setDate(Math.min(base.getDate(), daysInMonth(d.getFullYear(), d.getMonth())));
  return d;
}

export function EventCalendar({
  accounts,
  onOpen,
  weekStart,
  monthCount = 3,
  columns = 3,
  title,
  dividers = "columns",
}: {
  accounts: AccountListItem[];
  onOpen: (id: string) => void;
  weekStart: number;
  monthCount?: number;
  columns?: number;
  title?: string;
  dividers?: "none" | "columns" | "grid";
}) {
  const { t, i18n } = useTranslation();
  // Single-month + pager view kicks in at the tablet width too (not just phones):
  // the persistent sidebar leaves too little room for the 3-up grid. Matches the
  // 1300px tablet tier in index.css.
  const isMobile = useMediaQuery("(max-width: 1300px)");
  const [page, setPage] = useState(0); // which month is shown on mobile
  const now = new Date();
  const months = Array.from({ length: monthCount }, (_, off) => new Date(now.getFullYear(), now.getMonth() + off, 1));
  const rangeStart = new Date(now.getFullYear(), now.getMonth(), 1);
  const rangeEnd = new Date(now.getFullYear(), now.getMonth() + monthCount, 1); // exclusive

  const events = new Map<string, Ev[]>();
  const add = (key: string | null | undefined, ev: Ev) => {
    if (!key) return;
    events.set(key, [...(events.get(key) ?? []), ev]);
  };
  const inRange = (d: Date) => d >= rangeStart && d < rangeEnd;

  for (const a of accounts) {
    if (a.status !== "active") continue;
    // Payments: project every recurring occurrence into the visible window.
    if (a.next_payment_date) {
      const first = new Date(a.next_payment_date);
      const label = t("calendar.eventPayment", { service: a.service });
      if (a.billing_period_months > 0) {
        for (let k = 0; k < 400; k++) {
          const occ = addMonthsClamped(first, k * a.billing_period_months);
          if (occ >= rangeEnd) break;
          if (inRange(occ)) add(toKey(occ), { type: "payment", label, id: a.id });
        }
      } else if (inRange(first)) {
        add(toKey(first), { type: "payment", label, id: a.id });
      }
    }
    // Rotation: recurring every N days from the last change.
    if (a.rotate_every_days) {
      const label = t("calendar.eventRotation", { service: a.service });
      const due = new Date(a.password_changed_at);
      due.setDate(due.getDate() + a.rotate_every_days);
      // Fast-forward to the visible window, then step through it.
      if (due < rangeStart) {
        const jumps = Math.ceil((rangeStart.getTime() - due.getTime()) / (a.rotate_every_days * 86_400_000));
        due.setDate(due.getDate() + jumps * a.rotate_every_days);
      }
      for (let k = 0; k < 400 && due < rangeEnd; k++) {
        add(toKey(due), { type: "rotation", label, id: a.id });
        due.setDate(due.getDate() + a.rotate_every_days);
      }
    }
  }

  return (
    <div className="card" style={{ marginTop: 24 }}>
      <div className="topbar" style={{ marginBottom: 12 }}>
        <h2 className="section-title" style={{ margin: 0 }}>{title ?? t("calendar.title")}</h2>
        <div className="cal-legend">
          <span><i className="dot dot-payment" /> {t("calendar.payment")}</span>
          <span><i className="dot dot-rotation" /> {t("calendar.rotation")}</span>
        </div>
      </div>
      {isMobile && (
        <div className="cal-pager">
          <button type="button" className="cal-nav" onClick={() => setPage((p) => Math.max(0, p - 1))}
            disabled={page <= 0} aria-label={t("calendar.prevMonth")}><FaChevronLeft /></button>
          <span className="cal-pager-label">
            {months[Math.min(page, months.length - 1)].toLocaleString(i18n.language, { month: "long", year: "numeric" })}
          </span>
          <button type="button" className="cal-nav" onClick={() => setPage((p) => Math.min(months.length - 1, p + 1))}
            disabled={page >= months.length - 1} aria-label={t("calendar.nextMonth")}><FaChevronRight /></button>
        </div>
      )}
      <div className={`cal-row div-${isMobile ? "none" : dividers}`}
        style={{ gridTemplateColumns: `repeat(${isMobile ? 1 : columns}, minmax(0, 1fr))` }}>
        {(isMobile ? [months[Math.min(page, months.length - 1)]] : months).map((m) => (
          <MonthGrid key={toKey(m)} month={m} events={events} onOpen={onOpen} weekStart={weekStart} locale={i18n.language} isMobile={isMobile} />
        ))}
      </div>
    </div>
  );
}

// Short weekday names in the given locale (0 = Sunday). 2023-01-01 was a Sunday.
function weekdayNames(locale: string): string[] {
  const fmt = new Intl.DateTimeFormat(locale, { weekday: "short" });
  return Array.from({ length: 7 }, (_, i) => fmt.format(new Date(2023, 0, 1 + i)));
}

function MonthGrid({ month, events, onOpen, weekStart, locale, isMobile }: { month: Date; events: Map<string, Ev[]>; onOpen: (id: string) => void; weekStart: number; locale: string; isMobile: boolean }) {
  // On touch, tapping a day is explicit: one event opens it directly, several
  // toggle a bottom sheet of choices (hover isn't reliable on touch).
  const [openKey, setOpenKey] = useState<string | null>(null);
  const y = month.getFullYear();
  const mo = month.getMonth();
  // Leading blanks measured from the configured first day of the week.
  const leading = (new Date(y, mo, 1).getDay() - weekStart + 7) % 7;
  const days = new Date(y, mo + 1, 0).getDate();
  const todayKey = toKey(new Date());

  const cells: (number | null)[] = [];
  for (let i = 0; i < leading; i++) cells.push(null);
  for (let d = 1; d <= days; d++) cells.push(d);

  const names = weekdayNames(locale);
  const dow = Array.from({ length: 7 }, (_, i) => names[(i + weekStart) % 7]);

  // On mobile, tapping a day with events either opens it (single) or toggles
  // the sheet (multiple). Desktop keeps its hover tooltip (onClick is a no-op).
  function tapDay(key: string, evs: Ev[]) {
    if (!isMobile) return;
    if (evs.length === 1) { onOpen(evs[0].id); return; }
    setOpenKey((k) => (k === key ? null : key));
  }

  return (
    <div className="cal-month">
      <div className="cal-title">{month.toLocaleString(locale, { month: "long", year: "numeric" })}</div>
      {isMobile && openKey && <div className="cal-sheet-backdrop" onClick={() => setOpenKey(null)} />}
      <div className="cal-grid">
        {dow.map((w, i) => <div key={i} className="cal-dow">{w}</div>)}
        {cells.map((d, i) => {
          if (d === null) return <div key={i} className="cal-cell empty" />;
          const key = `${y}-${String(mo + 1).padStart(2, "0")}-${String(d).padStart(2, "0")}`;
          const evs = events.get(key);
          return (
            <div
              key={i}
              className={`cal-cell ${key === todayKey ? "today" : ""} ${evs ? "has-ev" : ""} ${openKey === key ? "open" : ""}`}
              onClick={evs && isMobile ? () => tapDay(key, evs) : undefined}
            >
              <span>{d}</span>
              {evs && (
                <span className="cal-dots">
                  {evs.slice(0, 3).map((e, j) => <i key={j} className={`dot dot-${e.type}`} />)}
                </span>
              )}
              {evs && (
                <div className="cal-tip" role="tooltip">
                  {evs.map((e, j) => (
                    <button key={j} type="button" className="cal-tip-row" onClick={() => onOpen(e.id)}>
                      <i className={`dot dot-${e.type}`} />
                      <span>{e.label}</span>
                    </button>
                  ))}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
