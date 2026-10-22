import i18n from "./i18n";
import { applyUpdate, createBackup, type VersionInfo } from "./api";
import { confirmWithCheckbox, errorAlert, toast } from "./alerts";

// Confirms and starts an in-app update. The dialog offers "Back up my data first"
// (ticked by default): the backup is made before anything is downloaded, and if it
// fails the update is not started. An upgraded database can't be opened by an older
// version, so a backup is the only way back. Shared by the banner and Settings.
export async function confirmAndApplyUpdate(info: VersionInfo, setBusy: (busy: boolean) => void): Promise<void> {
  const t = i18n.t.bind(i18n);
  const mac = info.os === "darwin";
  const r = await confirmWithCheckbox({
    title: t("update.confirmTitle", { version: info.latest }),
    text: mac ? t("update.confirmTextMac") : t("update.confirmText"),
    confirmText: t("update.updateNow"),
    checkboxLabel: t("update.backupFirst"),
    checked: true,
  });
  if (!r.confirmed) return;

  setBusy(true);
  try {
    if (r.checked) {
      try {
        await createBackup();
        toast(t("update.backupDone"));
      } catch (e) {
        errorAlert(t("update.backupFailed", { error: (e as Error).message }));
        return;
      }
    }
    await applyUpdate();
    toast(mac ? t("update.startedMac") : t("update.started"));
  } catch (e) {
    errorAlert((e as Error).message);
  } finally {
    setBusy(false);
  }
}
