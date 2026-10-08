import { useTranslation } from "react-i18next";
import { copySecret } from "../clipboard";
import { useLayout } from "./Layout";

// CopyText renders text that copies itself to the clipboard on click, showing a
// "Copied" toast (unified with the password tool's copy feedback). Copied values
// auto-clear from the clipboard per the Configuration setting.
export function CopyText({ value, mono = false }: { value: string; mono?: boolean }) {
  const { t } = useTranslation();
  const { clipboardClear } = useLayout();
  async function copy() {
    try {
      await copySecret(value, clipboardClear);
    } catch {
      /* clipboard may be blocked; ignore */
    }
  }

  return (
    <span className={`copyable ${mono ? "mono" : ""}`} onClick={copy} title={t("common.clickToCopy")}>
      {mono ? <code>{value}</code> : value}
    </span>
  );
}
