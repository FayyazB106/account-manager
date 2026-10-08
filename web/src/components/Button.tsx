import type { ButtonHTMLAttributes, ReactNode } from "react";

// Button — the single component for all rectangular buttons. Variants map to the
// app's button styles so callers never hand-write class combos.
type Variant = "primary" | "ghost" | "danger" | "warn" | "ok";

const VARIANT_CLASS: Record<Variant, string> = {
  primary: "btn-primary",
  ghost: "ghost",
  danger: "ghost danger-btn",
  warn: "ghost warn-btn",
  ok: "ghost ok-btn",
};

export function Button({
  variant = "primary",
  children,
  className = "",
  ...rest
}: { variant?: Variant; children: ReactNode } & ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button className={`${VARIANT_CLASS[variant]} ${className}`.trim()} {...rest}>
      {children}
    </button>
  );
}
