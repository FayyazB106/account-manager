import { useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import {
  getProfile, getSMTP, setMonthlyReport, setPaymentReminders, setRotationReminders, setUpcomingPayments, setBreachAlerts,
  sendTestEmail, sendReportPreview, sendReminderPreview, sendRotationPreview, sendUpcomingPreview, sendBreachAlertPreview,
} from "../api";
import { errorAlert, toast } from "../alerts";
import { Button } from "./Button";

type Busy = "" | "monthly" | "upcoming" | "reminders" | "rotation" | "breach" | "test"
  | "sendMonthly" | "sendUpcoming" | "sendReminders" | "sendRotation" | "sendBreach";

// SendEmailsCard is where an admin chooses which emails the app sends them: the monthly
// payment history (last month's spreadsheet) and the month's upcoming payments (both
// on the 1st), plus daily payment reminders and daily password-rotation reminders
// (each while something is due within 7 days or overdue), and an alert sent the moment
// a breach scan finds a newly breached password. It sits under Two-factor authentication in Settings → Admin and
// needs Settings → Email to be switched on and set up first.
export function SendEmailsCard() {
  const { t } = useTranslation();
  const [monthly, setMonthly] = useState(false);
  const [reminders, setReminders] = useState(false);
  const [rotation, setRotation] = useState(false);
  const [upcoming, setUpcoming] = useState(false);
  const [breach, setBreach] = useState(false);
  const [configured, setConfigured] = useState(false);
  const [emailOn, setEmailOn] = useState(false);
  const [busy, setBusy] = useState<Busy>("");

  useEffect(() => {
    getProfile().then((p) => {
      setMonthly(!!p.monthly_report);
      setReminders(!!p.payment_reminders);
      setRotation(!!p.rotation_reminders);
      setUpcoming(!!p.upcoming_payments);
      setBreach(!!p.breach_alerts);
    }).catch(() => {});
    getSMTP().then((s) => { setConfigured(s.configured); setEmailOn(s.enabled); }).catch(() => {});
  }, []);

  const ready = configured && emailOn;

  // Run an action with a busy flag and the shared error handling.
  async function run(flag: Busy, action: () => Promise<void>) {
    setBusy(flag);
    try { await action(); }
    catch (e) { errorAlert((e as Error).message); }
    finally { setBusy(""); }
  }

  const toggleMonthly = (on: boolean) => run("monthly", async () => {
    await setMonthlyReport(on);
    setMonthly(on);
    toast(on ? t("report.enabledToast") : t("report.disabledToast"));
  });
  const toggleReminders = (on: boolean) => run("reminders", async () => {
    await setPaymentReminders(on);
    setReminders(on);
    toast(on ? t("report.enabledToast") : t("report.disabledToast"));
  });
  const toggleUpcoming = (on: boolean) => run("upcoming", async () => {
    await setUpcomingPayments(on);
    setUpcoming(on);
    toast(on ? t("report.enabledToast") : t("report.disabledToast"));
  });
  const toggleRotation = (on: boolean) => run("rotation", async () => {
    await setRotationReminders(on);
    setRotation(on);
    toast(on ? t("report.enabledToast") : t("report.disabledToast"));
  });
  const toggleBreach = (on: boolean) => run("breach", async () => {
    await setBreachAlerts(on);
    setBreach(on);
    toast(on ? t("report.enabledToast") : t("report.disabledToast"));
  });
  const sendTest = () => run("test", async () => {
    const r = await sendTestEmail();
    toast(t("report.testSent", { email: r.sent_to }));
  });
  const sendMonthly = () => run("sendMonthly", async () => {
    const r = await sendReportPreview();
    toast(t("report.reportSent", { email: r.sent_to }));
  });
  const sendUpcoming = () => run("sendUpcoming", async () => {
    const r = await sendUpcomingPreview();
    toast(r.sent ? t("report.upcomingSent", { email: r.sent_to }) : t("report.upcomingNothing"));
  });
  const sendRotation = () => run("sendRotation", async () => {
    const r = await sendRotationPreview();
    toast(r.sent ? t("report.rotationSent", { email: r.sent_to }) : t("report.rotationNothing"));
  });
  const sendBreach = () => run("sendBreach", async () => {
    const r = await sendBreachAlertPreview();
    toast(r.sent ? t("report.breachSent", { email: r.sent_to }) : t("report.breachNothing"));
  });
  const sendReminders = () => run("sendReminders", async () => {
    const r = await sendReminderPreview();
    toast(r.sent ? t("report.reminderSent", { email: r.sent_to }) : t("report.reminderNothing"));
  });

  return (
    <div className="totp-section" id="s-send-emails">
      <div className="topbar" style={{ marginBottom: 8 }}>
        <h2 className="section-title" style={{ margin: 0 }}>{t("report.title")}</h2>
        <Button variant="ghost" onClick={sendTest} disabled={!ready || busy !== ""}>
          {busy === "test" ? t("report.sending") : t("report.sendTest")}
        </Button>
      </div>
      <p className="muted" style={{ marginTop: 0 }}>{t("report.intro")}</p>

      {/* While email is off (or not set up) the whole list is locked and dimmed:
          switches and Send Now alike. */}
      <fieldset className="email-fields mail-rows" disabled={!ready}>
      <EmailRow
        name={t("report.monthlyName")}
        desc={t("report.monthlyDesc")}
        checked={monthly}
        switchDisabled={busy !== ""}
        onToggle={toggleMonthly}
        sendLabel={busy === "sendMonthly" ? t("report.sending") : t("report.sendNow")}
        onSend={sendMonthly}
        sendDisabled={busy !== ""}
      />
      <EmailRow
        name={t("report.upcomingName")}
        desc={t("report.upcomingDesc")}
        checked={upcoming}
        switchDisabled={busy !== ""}
        onToggle={toggleUpcoming}
        sendLabel={busy === "sendUpcoming" ? t("report.sending") : t("report.sendNow")}
        onSend={sendUpcoming}
        sendDisabled={busy !== ""}
      />
      <EmailRow
        name={t("report.reminderName")}
        desc={t("report.reminderDesc")}
        checked={reminders}
        switchDisabled={busy !== ""}
        onToggle={toggleReminders}
        sendLabel={busy === "sendReminders" ? t("report.sending") : t("report.sendNow")}
        onSend={sendReminders}
        sendDisabled={busy !== ""}
      />

      <EmailRow
        name={t("report.rotationName")}
        desc={t("report.rotationDesc")}
        checked={rotation}
        switchDisabled={busy !== ""}
        onToggle={toggleRotation}
        sendLabel={busy === "sendRotation" ? t("report.sending") : t("report.sendNow")}
        onSend={sendRotation}
        sendDisabled={busy !== ""}
      />

      <EmailRow
        name={t("report.breachName")}
        desc={t("report.breachDesc")}
        checked={breach}
        switchDisabled={busy !== ""}
        onToggle={toggleBreach}
        sendLabel={busy === "sendBreach" ? t("report.sending") : t("report.sendNow")}
        onSend={sendBreach}
        sendDisabled={busy !== ""}
      />

      </fieldset>

      {!ready && (
        <p className="field-hint">
          {!emailOn ? t("report.emailOff") : t("report.needsEmail")}{" "}
          <Link to="/settings/email">{t("report.goToEmail")}</Link>
        </p>
      )}
    </div>
  );
}

// EmailRow is one email the admin can receive: a switch (with what it is) on the
// left and its "Send Now" button opposite it.
function EmailRow({
  name, desc, checked, switchDisabled, onToggle, sendLabel, onSend, sendDisabled,
}: {
  name: ReactNode;
  desc: ReactNode;
  checked: boolean;
  switchDisabled: boolean;
  onToggle: (on: boolean) => void;
  sendLabel: ReactNode;
  onSend: () => void;
  sendDisabled: boolean;
}) {
  return (
    <div className="mail-row">
      <div className="mail-row-text">
        <label className="switch-row">
          <input
            type="checkbox"
            role="switch"
            checked={checked}
            disabled={switchDisabled}
            onChange={(e) => onToggle(e.target.checked)}
          />
          <span>{name}</span>
        </label>
        <p className="field-hint" style={{ margin: "4px 0 0" }}>{desc}</p>
      </div>
      <Button variant="ghost" onClick={onSend} disabled={sendDisabled}>{sendLabel}</Button>
    </div>
  );
}
