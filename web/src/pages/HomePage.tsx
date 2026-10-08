import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useLayout } from "../components/Layout";
import { loadHealthCache } from "../health-scan";
import { generatePassword } from "../api";
import { passwordStrength, formatCrackTime } from "../strength";
import { pwnedCount } from "../pwned";
import { Button } from "../components/Button";
import { IconButton } from "../components/IconButton";
import { PasswordInput } from "../components/PasswordInput";
import { EventCalendar } from "../components/EventCalendar";
import { copySecret } from "../clipboard";
import { FaHeartbeat, FaMoneyBillWave, FaSyncAlt, FaShieldAlt, FaCopy } from "react-icons/fa";
import type { AccountListItem } from "../api";

function rotationDue(a: AccountListItem): Date | null {
  if (!a.rotate_every_days) return null;
  const d = new Date(a.password_changed_at);
  d.setDate(d.getDate() + a.rotate_every_days);
  return d;
}

function daysUntil(d: Date): number {
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const due = new Date(d);
  due.setHours(0, 0, 0, 0);
  return Math.round((due.getTime() - today.getTime()) / 86_400_000);
}

type TFunc = (key: string, opts?: Record<string, unknown>) => string;

function countdownLabel(t: TFunc, days: number): string {
  if (days < 0) return t("home.overdue", { count: Math.abs(days) });
  if (days === 0) return t("home.today");
  return t("home.inDays", { count: days });
}

export function HomePage() {
  const { accounts, weekStart, displayName } = useLayout();
  const { t } = useTranslation();
  const navigate = useNavigate();
  const health = loadHealthCache()?.report ?? null;

  // Soonest upcoming (or most overdue) payment.
  const nextPay = accounts
    .filter((a) => a.next_payment_date)
    .sort((a, b) => (a.next_payment_date! < b.next_payment_date! ? -1 : 1))[0];

  // Soonest password rotation due.
  const rotItems = accounts
    .filter((a) => a.rotate_every_days)
    .map((a) => ({ a, due: rotationDue(a)! }))
    .sort((x, y) => (x.due < y.due ? -1 : 1));
  const nextRot = rotItems[0];

  const payDays = nextPay ? daysUntil(new Date(nextPay.next_payment_date!)) : null;
  const rotDays = nextRot ? daysUntil(nextRot.due) : null;

  // How many accounts share the soonest payment / rotation day (for the tile hint).
  const payKey = nextPay?.next_payment_date?.slice(0, 10);
  const payCount = payKey ? accounts.filter((a) => a.next_payment_date?.slice(0, 10) === payKey).length : 0;
  const rotKey = nextRot?.due.toISOString().slice(0, 10);
  const rotCount = rotKey ? rotItems.filter((x) => x.due.toISOString().slice(0, 10) === rotKey).length : 0;
  const payHint = nextPay ? (payCount > 1 ? t("home.accountsCount", { count: payCount }) : nextPay.service) : t("home.noDatedPayments");
  const rotHint = nextRot ? (rotCount > 1 ? t("home.accountsCount", { count: rotCount }) : nextRot.a.service) : t("home.noRotationPolicies");

  return (
    <>
      <header className="topbar home-greeting"><h1>{displayName ? t("home.greeting", { name: displayName }) : t("home.title")}</h1></header>

      <div className="tiles">
        <Tile
          icon={<FaHeartbeat />}
          label={t("home.vaultHealth")}
          value={health ? <><span className={`score-${scoreBand(health.score)}`}>{health.score}</span><span className="tile-value-sub">/100</span></> : "—"}
          hint={health ? t("home.viewReport") : t("home.notScanned")}
          onClick={() => navigate("/health")}
        />
        <Tile
          icon={<FaMoneyBillWave />}
          label={t("home.nextPayment")}
          value={payDays !== null ? countdownLabel(t, payDays) : t("common.none")}
          hint={payHint}
          danger={payDays !== null && payDays <= 7}
          onClick={nextPay ? () => navigate(`/accounts?paydate=${nextPay.next_payment_date!.slice(0, 10)}`) : undefined}
        />
        <Tile
          icon={<FaSyncAlt />}
          label={t("home.nextRotation")}
          value={rotDays !== null ? countdownLabel(t, rotDays) : t("common.none")}
          hint={rotHint}
          danger={rotDays !== null && rotDays <= 7}
          onClick={nextRot ? () => navigate(`/accounts?rotdate=${nextRot.due.toISOString().slice(0, 10)}`) : undefined}
        />
        <Tile
          icon={<FaShieldAlt />}
          label={t("home.breached")}
          value={health ? String(health.pwnedCount) : "—"}
          hint={t("home.viewReport")}
          danger={!!health && health.pwnedCount > 0}
          onClick={() => navigate("/health")}
        />
      </div>

      <PasswordTool />

      <EventCalendar accounts={accounts} weekStart={weekStart} onOpen={(id) => navigate(`/account/${id}`)} />
    </>
  );
}

function Tile({
  icon, label, value, hint, onClick, danger,
}: {
  icon: React.ReactNode;
  label: string;
  value: React.ReactNode;
  hint?: string;
  onClick?: () => void;
  danger?: boolean;
}) {
  return (
    <div className={`tile card ${onClick ? "clickable" : ""}`} onClick={onClick}>
      <div className="tile-head"><span className="tile-icon">{icon}</span> {label}</div>
      <div className={`tile-value ${danger ? "danger-text" : ""}`}>{value}</div>
      {hint && <div className="muted tile-hint">{hint}</div>}
    </div>
  );
}

function scoreBand(score: number): "weak" | "fair" | "good" {
  if (score < 50) return "weak";
  if (score < 80) return "fair";
  return "good";
}

const PCT: Record<string, number> = { weak: 25, fair: 50, strong: 75, excellent: 100 };

function Donut({ pct, label, size = 140 }: { pct: number; label: string; size?: number }) {
  const sw = 14;
  const r = (size - sw) / 2 - 1;
  const c = 2 * Math.PI * r;
  const mid = size / 2;
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} className="donut">
      <circle cx={mid} cy={mid} r={r} className="donut-track" fill="none" strokeWidth={sw} />
      <circle
        cx={mid} cy={mid} r={r} fill="none" strokeWidth={sw}
        className={`donut-val sb-${label}`}
        strokeDasharray={`${(pct / 100) * c} ${c}`}
        transform={`rotate(-90 ${mid} ${mid})`}
      />
      <text x={mid} y={mid + 6} textAnchor="middle" className="donut-text" style={{ fontSize: size * 0.18 }}>
        {pct ? `${pct}%` : ""}
      </text>
    </svg>
  );
}

function PasswordTool() {
  const { t } = useTranslation();
  const { clipboardClear } = useLayout();
  const [length, setLength] = useState(20);
  const [pw, setPw] = useState("");
  const [pwned, setPwned] = useState<number | null>(null);

  // Debounced breach check (same k-anonymity call as the entry fields).
  useEffect(() => {
    if (!pw) { setPwned(null); return; }
    setPwned(null);
    const t = setTimeout(async () => setPwned(await pwnedCount(pw)), 500);
    return () => clearTimeout(t);
  }, [pw]);

  async function gen() {
    try { setPw((await generatePassword(length)).password); } catch { /* ignore */ }
  }

  function copy() {
    if (!pw) return;
    copySecret(pw, clipboardClear).catch(() => {});
  }

  const s = pw ? passwordStrength(pw) : null;
  const pct = s ? PCT[s.label] : 0;

  return (
    <div className="card pw-tool">
      <h2 className="section-title">{t("home.passwordTool")}</h2>

      <div className="pw-tool-grid">
        <div className="pw-tool-left">
          <div className="pw-tool-row">
            <PasswordInput value={pw} onChange={setPw} placeholder={t("home.typeOrGenerate")} />
            <IconButton title={t("common.copied")} onClick={copy}><FaCopy /></IconButton>
            <Button type="button" onClick={gen}>{t("home.generate")}</Button>
          </div>

          <label className="gen-len" style={{ marginTop: 16 }}>
            <span>{t("home.length", { count: length })}</span>
            <input type="range" min={8} max={64} value={length}
              style={{ ["--pct" as string]: `${((length - 8) / (64 - 8)) * 100}%` }}
              onChange={(e) => setLength(Number(e.target.value))} />
          </label>
        </div>

        <div className="pw-tool-right">
          <Donut pct={pct} label={s?.label ?? "none"} size={120} />
          <div className="pw-tool-meta">
            {s ? (
              <><span className={`pw-tool-strength strength-${s.label}`}>{t(`strength.${s.label}`)}</span> <span className="muted">· {t("home.cracksIn", { time: formatCrackTime(t, s.crackSeconds) })}</span></>
            ) : (
              <span className="muted">{t("home.enterPassword")}</span>
            )}
          </div>
          <div className="pw-tool-breach">
            {pwned !== null && pwned > 0 && (
              <span className="field-hint strength-weak">{t("home.foundInBreaches", { count: pwned.toLocaleString() })}</span>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
