import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

// Badge — a small status pill. Tone controls the color.
export type BadgeTone = "excellent" | "strong" | "fair" | "weak";

export function Badge({ tone, children, title }: { tone: BadgeTone; children: ReactNode; title?: string }) {
  return <span className={`badge badge-${tone}`} title={title}>{children}</span>;
}

// StatusBadge — a colored dot for account active/inactive status.
export function StatusBadge({ status }: { status: string }) {
  const { t } = useTranslation();
  const active = status === "active";
  const label = active ? t("accounts.active") : t("accounts.inactive");
  return <span className={`status-dot ${active ? "status-active" : "status-inactive"}`} title={label} aria-label={label} />;
}
