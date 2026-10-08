import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { getSettings, setSetting } from "../api";
import { errorAlert, toast } from "../alerts";
import { Button } from "../components/Button";

// DeploymentSettings configures how the server binds: the port, whether it's
// reachable from the network (localhost vs 0.0.0.0), and its public URL. These
// are read at startup, so changes apply after a restart. The app only listens on
// a host:port; a reverse proxy in front maps the domain and handles TLS.
export function DeploymentSettings() {
  const { t } = useTranslation();
  const [port, setPort] = useState("8080");
  const [network, setNetwork] = useState("off");
  const [publicUrl, setPublicUrl] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    getSettings().then((s) => {
      setPort(s.deploy_port || "8080");
      setNetwork(s.deploy_network || "off");
      setPublicUrl(s.deploy_public_url || "");
    }).catch(() => {});
  }, []);

  async function save() {
    const p = Number(port);
    if (!Number.isInteger(p) || p < 1 || p > 65535) {
      errorAlert(t("deploy.portInvalid"));
      return;
    }
    setBusy(true);
    try {
      await setSetting("deploy_port", String(p));
      await setSetting("deploy_network", network);
      await setSetting("deploy_public_url", publicUrl.trim());
      toast(t("deploy.savedRestart"));
    } catch (e) {
      errorAlert((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="card">
      <h2 className="section-title">{t("deploy.title")}</h2>
      <p className="muted">{t("deploy.intro")}</p>

      <div className="deploy-field">
        <label className="field-label" htmlFor="dep-port">{t("deploy.port")}</label>
        <input id="dep-port" type="number" min={1} max={65535} dir="ltr" value={port}
          onChange={(e) => setPort(e.target.value)} style={{ maxWidth: 160 }} />
        <p className="field-hint">{t("deploy.portHint")}</p>
      </div>

      <div className="deploy-field">
        <label className="field-label" htmlFor="dep-net">{t("deploy.network")}</label>
        <select id="dep-net" dir="ltr" value={network}
          onChange={(e) => setNetwork(e.target.value)} style={{ maxWidth: 340 }}>
          <option value="off">{t("deploy.networkOff")}</option>
          <option value="on">{t("deploy.networkOn")}</option>
        </select>
        <p className="field-hint">{t("deploy.networkHint")}</p>
      </div>

      {network === "on" && (
        <div className="deploy-field">
          <label className="field-label" htmlFor="dep-url">{t("deploy.publicUrl")}</label>
          <input id="dep-url" dir="ltr" value={publicUrl} placeholder="https://accounts.example.com"
            onChange={(e) => setPublicUrl(e.target.value)} style={{ maxWidth: 360 }} />
          <p className="field-hint">{t("deploy.publicUrlHint")}</p>
        </div>
      )}

      <div style={{ marginTop: 8, display: "flex", alignItems: "center", gap: 12 }}>
        <Button onClick={save} disabled={busy}>{t("common.save")}</Button>
        <span className="muted">{t("deploy.restartNote")}</span>
      </div>
    </div>
  );
}
