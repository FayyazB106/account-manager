import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";

export interface ContextMenuItem {
  label: string;
  icon?: ReactNode;
  onSelect: () => void;
}

// ContextMenu is a small right-click menu drawn at the pointer. It closes on any click
// elsewhere, on Escape, when the page scrolls or resizes, and when another context menu
// is opened. It is kept inside the viewport.
export function ContextMenu({ x, y, items, onClose }: { x: number; y: number; items: ContextMenuItem[]; onClose: () => void }) {
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ left: x, top: y });

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    setPos({
      left: Math.max(4, Math.min(x, window.innerWidth - r.width - 4)),
      top: Math.max(4, Math.min(y, window.innerHeight - r.height - 4)),
    });
  }, [x, y]);

  useEffect(() => {
    const close = () => onClose();
    const onDown = (e: MouseEvent) => { if (!ref.current?.contains(e.target as Node)) onClose(); };
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    window.addEventListener("scroll", close, true);
    window.addEventListener("resize", close);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
      window.removeEventListener("scroll", close, true);
      window.removeEventListener("resize", close);
    };
  }, [onClose]);

  return (
    <div ref={ref} className="ctx-menu" role="menu" style={{ left: pos.left, top: pos.top }}>
      {items.map((it) => (
        <button key={it.label} type="button" role="menuitem" className="ctx-item"
          onClick={() => { onClose(); it.onSelect(); }}>
          {it.icon}
          <span>{it.label}</span>
        </button>
      ))}
    </div>
  );
}
