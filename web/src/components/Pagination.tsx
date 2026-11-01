import { useState } from "react";
import { useTranslation } from "react-i18next";
import { FaChevronLeft, FaChevronRight } from "react-icons/fa";

export const PAGE_SIZES = [10, 25, 50, 100];
const SIZE_KEY = "am_page_size";

function readSize(): number {
  try {
    const n = Number(localStorage.getItem(SIZE_KEY));
    if (PAGE_SIZES.includes(n)) return n;
  } catch { /* storage unavailable */ }
  return 25;
}

export interface PageState<T> {
  items: T[];     // the rows to show on the current page
  page: number;   // 1-based, already clamped to a valid page
  pages: number;
  size: number;
  total: number;  // rows across all pages
  setPage: (n: number) => void;
  setSize: (n: number) => void;
}

// usePagination slices a list into pages. `resetKey` is any string that changes when the
// list's filters or sorting change; the view then jumps back to page 1 (a page that no
// longer exists after the list shrinks is clamped automatically). The page size is
// remembered per browser and shared by every list.
export function usePagination<T>(list: T[], resetKey = ""): PageState<T> {
  const [page, setPageRaw] = useState(1);
  const [size, setSizeRaw] = useState(readSize);
  const [seenKey, setSeenKey] = useState(resetKey);
  if (seenKey !== resetKey) { setSeenKey(resetKey); setPageRaw(1); }

  const pages = Math.max(1, Math.ceil(list.length / size));
  const cur = Math.min(page, pages);
  return {
    items: list.slice((cur - 1) * size, cur * size),
    page: cur,
    pages,
    size,
    total: list.length,
    setPage: (n) => setPageRaw(Math.min(Math.max(1, n), pages)),
    setSize: (n) => {
      setSizeRaw(n);
      setPageRaw(1);
      try { localStorage.setItem(SIZE_KEY, String(n)); } catch { /* ignore */ }
    },
  };
}

// The page numbers to show: always the first and last, the current page and its
// neighbours, with "…" where numbers are skipped.
function pageNumbers(page: number, pages: number): (number | "gap")[] {
  const keep = new Set([1, pages, page - 1, page, page + 1]);
  const out: (number | "gap")[] = [];
  let last = 0;
  for (let n = 1; n <= pages; n++) {
    if (!keep.has(n)) continue;
    if (n - last > 1) out.push("gap");
    out.push(n);
    last = n;
  }
  return out;
}

// Pagination is the footer under a paged list: "1–25 of 140", previous/next with page
// numbers, and a page-size picker. Hidden while everything fits on the smallest page.
export function Pagination<T>({ p }: { p: PageState<T> }) {
  const { t } = useTranslation();
  if (p.total <= PAGE_SIZES[0]) return null;
  const from = (p.page - 1) * p.size + 1;
  const to = Math.min(p.page * p.size, p.total);

  return (
    <div className="pager">
      <span className="muted pager-range" dir="ltr">{t("pagination.showing", { from, to, total: p.total })}</span>
      <nav className="pager-nav" aria-label={t("pagination.label")}>
        <button type="button" className="pager-btn pager-arrow" disabled={p.page <= 1}
          title={t("pagination.prev")} aria-label={t("pagination.prev")} onClick={() => p.setPage(p.page - 1)}>
          <FaChevronLeft size={12} />
        </button>
        {pageNumbers(p.page, p.pages).map((n, i) =>
          n === "gap" ? (
            <span key={`g${i}`} className="pager-gap">…</span>
          ) : (
            <button key={n} type="button" className={`pager-btn ${n === p.page ? "active" : ""}`.trim()}
              aria-current={n === p.page ? "page" : undefined} onClick={() => p.setPage(n)}>
              {n}
            </button>
          ),
        )}
        <button type="button" className="pager-btn pager-arrow" disabled={p.page >= p.pages}
          title={t("pagination.next")} aria-label={t("pagination.next")} onClick={() => p.setPage(p.page + 1)}>
          <FaChevronRight size={12} />
        </button>
      </nav>
      <label className="pager-size muted">
        <span>{t("pagination.perPage")}</span>
        <select dir="ltr" value={p.size} onChange={(e) => p.setSize(Number(e.target.value))}>
          {PAGE_SIZES.map((n) => <option key={n} value={n}>{n}</option>)}
        </select>
      </label>
    </div>
  );
}
