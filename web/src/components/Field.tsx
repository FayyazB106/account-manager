import type { ReactNode } from "react";

// Field — a labeled form control wrapper. Standardizes the label + optional hint
// around any control (input, select, textarea, or a composite) passed as
// children, so every form field looks and spaces the same.
export function Field({
  label,
  hint,
  full = false,
  group = false,
  className = "",
  children,
}: {
  label: ReactNode;
  hint?: ReactNode;
  full?: boolean; // span both columns in a 2-col grid
  // For a field that holds several controls (a list of seats, an amount + currency). A
  // <label> would focus its FIRST control whenever you click anywhere inside it, so these
  // render as a plain group instead.
  group?: boolean;
  className?: string;
  children: ReactNode;
}) {
  const cls = `${group ? "field-group" : ""} ${full ? "full-field" : ""} ${className}`.trim();
  const inner = (
    <>
      <span className="field-label">{label}</span>
      {children}
      {hint && <span className="field-hint">{hint}</span>}
    </>
  );
  return group ? <div role="group" className={cls}>{inner}</div> : <label className={cls}>{inner}</label>;
}
