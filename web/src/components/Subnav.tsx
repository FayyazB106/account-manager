import { useEffect, useRef, useState, type ReactNode } from "react";

// Subnav wraps the settings/finance sub-navigation. On mobile the nav becomes a
// horizontal scroller; this wrapper shows left/right fades (like the sidebar's
// category list) that appear only when there's more to scroll in that direction.
export function Subnav({ title, children, className = "" }: { title: string; children: ReactNode; className?: string }) {
  const ref = useRef<HTMLElement>(null);
  const [fade, setFade] = useState({ left: false, right: false });

  function update() {
    const el = ref.current;
    if (!el) return;
    setFade({
      left: el.scrollLeft > 4,
      right: el.scrollLeft + el.clientWidth < el.scrollWidth - 4,
    });
  }

  useEffect(() => {
    update();
    window.addEventListener("resize", update);
    return () => window.removeEventListener("resize", update);
  }, [children]);

  return (
    <div className={`subnav-wrap ${fade.left ? "fade-left" : ""} ${fade.right ? "fade-right" : ""} ${className}`.trim()}>
      <nav className="subnav" ref={ref} onScroll={update}>
        <div className="subnav-title">{title}</div>
        {children}
      </nav>
    </div>
  );
}
