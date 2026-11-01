import { useState } from "react";
import { useTranslation } from "react-i18next";
import { celebrationEnabled, setCelebrationEnabled, celebratePayment, type CelebrationEffect } from "../celebrate";

// Two switches for the "Mark as paid" effects. They are per browser (like the theme), so
// each person or device chooses for themselves. Turning one on plays it once as a preview.
export function CelebrationSettings() {
  const { t } = useTranslation();
  const [sound, setSound] = useState(celebrationEnabled("sound"));
  const [confetti, setConfetti] = useState(celebrationEnabled("confetti"));

  function change(effect: CelebrationEffect, on: boolean) {
    setCelebrationEnabled(effect, on);
    if (effect === "sound") setSound(on); else setConfetti(on);
    if (on) celebratePayment(effect); // a short preview of just that effect
  }

  return (
    <div className="totp-section" id="s-celebration" style={{ marginTop: 20 }}>
      <h2 className="section-title" style={{ marginBottom: 4 }}>{t("celebrate.title")}</h2>
      <p className="muted" style={{ marginTop: 0 }}>{t("celebrate.desc")}</p>
      <div>
        <label className="switch-row">
          <input type="checkbox" role="switch" checked={sound} onChange={(e) => change("sound", e.target.checked)} />
          <span>{t("celebrate.sound")}</span>
        </label>
      </div>
      <div style={{ marginTop: 10 }}>
        <label className="switch-row">
          <input type="checkbox" role="switch" checked={confetti} onChange={(e) => change("confetti", e.target.checked)} />
          <span>{t("celebrate.confetti")}</span>
        </label>
      </div>
    </div>
  );
}
