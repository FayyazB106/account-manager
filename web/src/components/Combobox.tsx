import { useEffect, useRef, useState, type KeyboardEvent } from "react";

// An option can be a plain string, or an object with a separate display label
// and extra search text (e.g. an Arabic name) — picking it yields `value`.
export type ComboOption = string | { value: string; label?: string; search?: string };

interface NormOption { value: string; label: string; search: string; }

function norm(o: ComboOption): NormOption {
  if (typeof o === "string") return { value: o, label: o, search: o };
  return { value: o.value, label: o.label ?? o.value, search: o.search ?? o.label ?? o.value };
}

// Combobox — a styled type-to-filter dropdown (replaces the unstylable native
// <datalist>). Lets you pick a suggestion or type a free value.
export function Combobox({
  value,
  onChange,
  options,
  placeholder,
  dir,
}: {
  value: string;
  onChange: (v: string) => void;
  options: ComboOption[];
  placeholder?: string;
  dir?: "rtl" | "ltr";
}) {
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1); // keyboard-highlighted option
  const ref = useRef<HTMLDivElement>(null);
  const listRef = useRef<HTMLUListElement>(null);

  useEffect(() => {
    function onDoc(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, []);

  const q = value.trim().toLowerCase();
  // Empty query → show the full list (so clicking/focusing opens the dropdown);
  // once something is typed, filter by it. Capped at 50 either way.
  const all = options.map(norm);
  const filtered = (q ? all.filter((o) => o.search.toLowerCase().includes(q)) : all).slice(0, 50);

  // Reset the highlight whenever the suggestions change or the list closes.
  useEffect(() => { setActive(-1); }, [q, open]);
  // Keep the highlighted option scrolled into view.
  useEffect(() => {
    if (active >= 0 && listRef.current) {
      (listRef.current.children[active] as HTMLElement | undefined)?.scrollIntoView({ block: "nearest" });
    }
  }, [active]);

  function pick(v: string) { onChange(v); setOpen(false); }

  function onKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      if (!open) { setOpen(true); return; }
      setActive((a) => Math.min(a + 1, filtered.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActive((a) => Math.max(a - 1, 0));
    } else if (e.key === "Enter") {
      if (open && active >= 0 && active < filtered.length) {
        e.preventDefault();
        pick(filtered[active].value);
      }
    } else if (e.key === "Escape") {
      setOpen(false);
    }
  }

  return (
    <div className="combo" ref={ref}>
      <input
        value={value}
        placeholder={placeholder}
        dir={dir}
        onChange={(e) => { onChange(e.target.value); setOpen(true); }}
        onFocus={() => setOpen(true)}
        onKeyDown={onKeyDown}
        role="combobox"
        aria-expanded={open && filtered.length > 0}
        aria-autocomplete="list"
      />
      {open && filtered.length > 0 && (
        <ul className="combo-list" ref={listRef} role="listbox">
          {filtered.map((o, i) => (
            <li
              key={o.value}
              role="option"
              aria-selected={i === active}
              className={i === active ? "active" : ""}
              onMouseEnter={() => setActive(i)}
              onMouseDown={() => pick(o.value)}
            >
              {o.label}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
