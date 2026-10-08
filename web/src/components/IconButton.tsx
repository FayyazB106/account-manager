import type { ReactNode } from "react";

// IconButton — a small borderless icon action (reveal, download, delete, edit…).
export function IconButton({
  children,
  onClick,
  title,
  danger = false,
  type = "button",
}: {
  children: ReactNode;
  onClick?: () => void;
  title?: string;
  danger?: boolean;
  type?: "button" | "submit";
}) {
  return (
    <button type={type} className={`icon-link ${danger ? "danger" : ""}`.trim()} title={title} onClick={onClick}>
      {children}
    </button>
  );
}
