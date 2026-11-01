// alerts.ts — thin wrappers around SweetAlert2 with our dark theme, so the app
// never uses the browser's native alert()/confirm() dialogs.

import Swal from "sweetalert2";
import i18n from "./i18n";
import { theme } from "./theme";

// Read the themed colors at call time so dialogs follow the active (dark/light) theme.
const base = () => ({
  background: theme.surface,
  color: theme.text,
});

export function errorAlert(message: string) {
  Swal.fire({ ...base(), icon: "error", title: i18n.t("common.error"), text: message, confirmButtonColor: theme.accent });
}

export async function confirmDialog(opts: {
  title: string;
  text: string;
  confirmText?: string;
  danger?: boolean; // red (destructive)
  warn?: boolean; // orange (caution, e.g. deactivate)
}): Promise<boolean> {
  const accent = opts.danger ? theme.danger : opts.warn ? theme.warn : theme.accent;
  const r = await Swal.fire({
    ...base(),
    icon: "warning",
    title: opts.title,
    text: opts.text,
    showCancelButton: true,
    confirmButtonText: opts.confirmText ?? i18n.t("common.confirm"),
    cancelButtonText: i18n.t("common.cancel"),
    confirmButtonColor: accent,
    cancelButtonColor: theme.borderStrong,
    iconColor: opts.danger ? theme.danger : theme.warn,
  });
  return r.isConfirmed;
}

// Like confirmDialog, plus one checkbox (e.g. "Back up first"). Resolves with whether
// the user confirmed and whether the box was ticked.
export async function confirmWithCheckbox(opts: {
  title: string;
  text: string;
  confirmText?: string;
  checkboxLabel: string;
  checked?: boolean;
}): Promise<{ confirmed: boolean; checked: boolean }> {
  const r = await Swal.fire({
    ...base(),
    icon: "warning",
    title: opts.title,
    text: opts.text,
    input: "checkbox",
    inputValue: opts.checked === false ? 0 : 1,
    inputPlaceholder: opts.checkboxLabel,
    showCancelButton: true,
    confirmButtonText: opts.confirmText ?? i18n.t("common.confirm"),
    cancelButtonText: i18n.t("common.cancel"),
    confirmButtonColor: theme.accent,
    cancelButtonColor: theme.borderStrong,
    iconColor: theme.warn,
  });
  return { confirmed: r.isConfirmed, checked: !!r.value };
}

// A plain warning with a single OK button (nothing to confirm — just "you can't do that").
export function warnDialog(title: string, text: string) {
  return Swal.fire({
    ...base(),
    icon: "warning",
    title,
    text,
    confirmButtonColor: theme.accent,
    iconColor: theme.warn,
  });
}

// A brief top-right success toast (used for "copied", etc.).
export function toast(message: string) {
  const rtl = document.documentElement.dir === "rtl";
  Swal.fire({
    ...base(),
    toast: true,
    // Keep the toast in the same visual corner (top-right) in both languages —
    // "end" resolves to the left in RTL, so flip to "start" there.
    position: rtl ? "top-start" : "top-end",
    timer: 1400,
    showConfirmButton: false,
    icon: "success",
    title: message,
  });
}
