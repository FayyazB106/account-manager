import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import jsQR from "jsqr";
import { Button } from "./Button";

// QRScanner opens the device camera and decodes a QR code, calling onResult with
// its text (e.g. an otpauth:// URI). Camera access needs a secure context (HTTPS
// or localhost) — over plain HTTP it reports that clearly instead of failing
// silently. Prefers the rear camera on phones.
export function QRScanner({ onResult, onClose }: { onResult: (text: string) => void; onClose: () => void }) {
  const { t } = useTranslation();
  const videoRef = useRef<HTMLVideoElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const rafRef = useRef<number | undefined>(undefined);
  const onResultRef = useRef(onResult);
  onResultRef.current = onResult;
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    function scan() {
      const video = videoRef.current, canvas = canvasRef.current;
      if (cancelled || !video || !canvas) return;
      if (video.readyState === video.HAVE_ENOUGH_DATA && video.videoWidth > 0) {
        const w = video.videoWidth, h = video.videoHeight;
        canvas.width = w; canvas.height = h;
        const ctx = canvas.getContext("2d", { willReadFrequently: true });
        if (ctx) {
          ctx.drawImage(video, 0, 0, w, h);
          const img = ctx.getImageData(0, 0, w, h);
          const found = jsQR(img.data, w, h, { inversionAttempts: "dontInvert" });
          if (found && found.data) {
            cancelled = true;
            onResultRef.current(found.data);
            return;
          }
        }
      }
      rafRef.current = requestAnimationFrame(scan);
    }

    async function start() {
      if (!window.isSecureContext || !navigator.mediaDevices?.getUserMedia) {
        setError(t("qr.insecure"));
        return;
      }
      try {
        const stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: { ideal: "environment" } } });
        if (cancelled) { stream.getTracks().forEach((tr) => tr.stop()); return; }
        streamRef.current = stream;
        const video = videoRef.current;
        if (!video) return;
        video.srcObject = stream;
        await video.play();
        scan();
      } catch (e) {
        const name = (e as DOMException).name;
        if (name === "NotAllowedError") setError(t("qr.denied"));
        else if (name === "NotFoundError" || name === "OverconstrainedError") setError(t("qr.noCamera"));
        else setError(t("qr.failed"));
      }
    }

    start();
    return () => {
      cancelled = true;
      if (rafRef.current) cancelAnimationFrame(rafRef.current);
      streamRef.current?.getTracks().forEach((tr) => tr.stop());
    };
  }, [t]);

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal qr-modal" onClick={(e) => e.stopPropagation()}>
        <h2 className="section-title">{t("qr.title")}</h2>
        {error ? (
          <p className="error">{error}</p>
        ) : (
          <>
            <p className="muted" style={{ marginTop: 0 }}>{t("qr.hint")}</p>
            <div className="qr-video-wrap">
              <video ref={videoRef} className="qr-video" playsInline muted />
              <div className="qr-frame" />
            </div>
          </>
        )}
        <canvas ref={canvasRef} style={{ display: "none" }} />
        <div className="form-actions" style={{ marginTop: 12, justifyContent: "center" }}>
          <Button variant="ghost" onClick={onClose}>{t("common.cancel")}</Button>
        </div>
      </div>
    </div>
  );
}
