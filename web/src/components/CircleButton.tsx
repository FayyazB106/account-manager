import type { ReactNode } from "react";

// CircleButton — the single component for every circular icon button in the app
// (add, upload, scan, category new/close, …). Keeps them consistent and makes
// restyling a one-file change.
export function CircleButton({
  children,
  onClick,
  title,
  variant = "solid",
  size = "md",
  open = false,
  disabled = false,
  type = "button",
  as = "button",
}: {
  children: ReactNode;
  onClick?: () => void;
  title?: string;
  variant?: "solid" | "ghost" | "ok";
  size?: "sm" | "md";
  open?: boolean; // rotates the icon 45° (used by the add toggle: + ↔ ×)
  disabled?: boolean;
  type?: "button" | "submit";
  as?: "button" | "label"; // "label" wraps a hidden <input> passed as children (file upload)
}) {
  const cls = ["fab", size === "sm" ? "sm" : "", variant !== "solid" ? variant : "", open ? "open" : ""]
    .filter(Boolean)
    .join(" ");

  if (as === "label") {
    return (
      <label className={cls} title={title} style={disabled ? { opacity: 0.6 } : undefined}>
        {children}
      </label>
    );
  }
  return (
    <button type={type} className={cls} title={title} disabled={disabled} onClick={onClick}>
      {children}
    </button>
  );
}
