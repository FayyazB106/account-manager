import i18n from "./i18n";
import { toast } from "./alerts";

// copySecret copies a secret to the clipboard and, if clearSeconds > 0, wipes it
// after that delay so a copied password doesn't linger. It shows a toast noting
// the auto-clear.
//
// Browser limitation: a page can only write to the clipboard while its tab is
// focused, so the clear is best-effort — if the user switches tabs before the
// timer fires, the browser may block the wipe. It still shrinks the exposure
// window versus leaving the secret in the clipboard indefinitely.

let clearTimer: ReturnType<typeof setTimeout> | undefined;

export async function copySecret(value: string, clearSeconds: number): Promise<void> {
  await navigator.clipboard.writeText(value);

  // Reset any pending clear from a previous copy.
  if (clearTimer) clearTimeout(clearTimer);
  clearTimer = undefined;

  if (clearSeconds > 0) {
    toast(i18n.t("common.copiedClears", { n: clearSeconds }));
    clearTimer = setTimeout(() => {
      // Best-effort wipe; ignore if the browser blocks it (tab not focused).
      navigator.clipboard.writeText("").catch(() => {});
      clearTimer = undefined;
    }, clearSeconds * 1000);
  } else {
    toast(i18n.t("common.copied"));
  }
}
