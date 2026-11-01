import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { useLayout } from "../components/Layout";
import { allAccountTOTPCodes } from "../api";
import { copySecret } from "../clipboard";
import { Icon } from "../icons";
import { CountdownRing } from "../components/CountdownRing";
import { Pagination, usePagination } from "../components/Pagination";

// AuthenticatorPage lists live 2FA codes for every account that has a stored
// TOTP secret, searchable by service/login. One bulk request per 30s window;
// a shared countdown ticks down and refetches when the window rolls over.
export function AuthenticatorPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { accounts, clipboardClear } = useLayout();
  const [codes, setCodes] = useState<Record<string, { code: string; expires_in: number }>>({});
  const [remaining, setRemaining] = useState(30);
  const [q, setQ] = useState("");
  const timer = useRef<ReturnType<typeof setInterval> | undefined>(undefined);

  const withTOTP = accounts.filter((a) => a.has_totp);

  useEffect(() => {
    let alive = true;
    async function load() {
      try {
        const c = await allAccountTOTPCodes();
        if (!alive) return;
        setCodes(c);
        const first = Object.values(c)[0];
        if (first) setRemaining(first.expires_in);
      } catch { /* ignore */ }
    }
    load();
    timer.current = setInterval(() => {
      setRemaining((s) => {
        if (s <= 1) { load(); return 30; }
        return s - 1;
      });
    }, 1000);
    return () => { alive = false; if (timer.current) clearInterval(timer.current); };
  }, []);

  const filtered = withTOTP.filter((a) => {
    const s = q.trim().toLowerCase();
    if (!s) return true;
    return a.service.toLowerCase().includes(s) || (a.login ?? "").toLowerCase().includes(s);
  });

  const pager = usePagination(filtered, q.trim().toLowerCase());

  return (
    <div className="card">
      <div className="topbar" style={{ marginBottom: 12 }}>
        <h1>{t("authenticator.title")}</h1>
        {withTOTP.length > 0 && <span title={t("accountTotp.expiresIn")}><CountdownRing seconds={remaining} /></span>}
      </div>

      {withTOTP.length === 0 ? (
        <p className="muted">{t("authenticator.empty")}</p>
      ) : (
        <>
          <input
            className="auth-search"
            placeholder={t("authenticator.search")}
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
          <ul className="auth-list">
            {pager.items.map((a) => {
              const c = codes[a.id];
              return (
                <li key={a.id} className="auth-row">
                  <span className="auth-svc clickable-row" onClick={() => navigate(`/account/${a.id}`)}>
                    {a.category_icon && <span className="cat-tag"><Icon name={a.category_icon} size={14} /></span>}
                    <span className="auth-svc-text">
                      <span className="auth-svc-name">{a.service}</span>
                      {a.login && <span className="muted auth-login">{a.login}</span>}
                    </span>
                  </span>
                  <code
                    className="acct-totp-code auth-code"
                    dir="ltr"
                    title={t("common.clickToCopy")}
                    onClick={() => c && copySecret(c.code, clipboardClear)}
                  >
                    {c ? `${c.code.slice(0, 3)} ${c.code.slice(3)}` : "······"}
                  </code>
                </li>
              );
            })}
            {filtered.length === 0 && <li className="muted">{t("authenticator.noMatch")}</li>}
          </ul>
          <Pagination p={pager} />
        </>
      )}
    </div>
  );
}
