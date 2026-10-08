import type { ReactNode } from "react";

// Field — a labeled form control wrapper. Standardizes the label + optional hint
// around any control (input, select, textarea, or a composite) passed as
// children, so every form field looks and spaces the same.
export function Field({
  label,
  hint,
  full = false,
  className = "",
  children,
}: {
  label: ReactNode;
  hint?: ReactNode;
  full?: boolean; // span both columns in a 2-col grid
  className?: string;
  children: ReactNode;
}) {
  return (
    <label className={`${full ? "full-field" : ""} ${className}`.trim()}>
      <span className="field-label">{label}</span>
      {children}
      {hint && <span className="field-hint">{hint}</span>}
    </label>
  );
}
