import { theme } from "./theme";

// A small reward for "Mark as paid": a cash-register "cha-ching" and a burst of green
// "$" confetti (the colour of the Mark as paid button). The sound is the bundled
// public/sounds/cha-ching.mp3; if it can't be played (blocked, missing, no audio
// support) a short synthesised cha-ching is used instead. "Reduce motion" skips just
// the confetti.

// Per-browser switches (Settings -> Admin). Both default to on.
export type CelebrationEffect = "sound" | "confetti";
const PREF_KEYS: Record<CelebrationEffect, string> = {
  sound: "am_celebrate_sound",
  confetti: "am_celebrate_confetti",
};

export function celebrationEnabled(effect: CelebrationEffect): boolean {
  try { return localStorage.getItem(PREF_KEYS[effect]) !== "0"; } catch { return true; }
}

export function setCelebrationEnabled(effect: CelebrationEffect, on: boolean) {
  try { localStorage.setItem(PREF_KEYS[effect], on ? "1" : "0"); } catch { /* storage unavailable */ }
}

const SOUND_URL = "/sounds/cha-ching.mp3";
let audio: HTMLAudioElement | null = null;

// preloadCelebration fetches the sound ahead of time (called when the Mark as paid
// form opens) so it plays the instant the payment is saved.
export function preloadCelebration() {
  if (!celebrationEnabled("sound")) return;
  try {
    if (!audio) {
      audio = new Audio(SOUND_URL);
      audio.preload = "auto";
      audio.volume = 0.7;
    }
  } catch { /* no audio */ }
}

function playSound() {
  if (!celebrationEnabled("sound")) return;
  try {
    preloadCelebration();
    if (!audio) { synthChaChing(); return; }
    audio.currentTime = 0;
    audio.play().catch(() => synthChaChing());
  } catch { synthChaChing(); }
}

// Fallback: a short synthesised cha-ching, used only if the mp3 can't be played.
function synthChaChing() {
  try {
    const Ctx = window.AudioContext ?? (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
    if (!Ctx) return;
    const ctx = new Ctx();
    const now = ctx.currentTime;
    const master = ctx.createGain();
    master.gain.value = 0.25;
    master.connect(ctx.destination);

    // "Ka": a short burst of filtered noise (the drawer slamming open).
    const len = Math.floor(ctx.sampleRate * 0.06);
    const buf = ctx.createBuffer(1, len, ctx.sampleRate);
    const data = buf.getChannelData(0);
    for (let i = 0; i < len; i++) data[i] = (Math.random() * 2 - 1) * (1 - i / len);
    const noise = ctx.createBufferSource();
    noise.buffer = buf;
    const band = ctx.createBiquadFilter();
    band.type = "bandpass";
    band.frequency.value = 1800;
    noise.connect(band).connect(master);
    noise.start(now);

    // "Ching": a bright bell — a few high partials that ring out and fade.
    [[1568, 1], [2349, 0.6], [3136, 0.4], [4699, 0.2]].forEach(([freq, level]) => {
      const osc = ctx.createOscillator();
      const g = ctx.createGain();
      osc.type = "sine";
      osc.frequency.value = freq;
      g.gain.setValueAtTime(0.0001, now + 0.07);
      g.gain.exponentialRampToValueAtTime(level, now + 0.085);
      g.gain.exponentialRampToValueAtTime(0.0001, now + 1.1);
      osc.connect(g).connect(master);
      osc.start(now + 0.07);
      osc.stop(now + 1.15);
    });
    window.setTimeout(() => ctx.close().catch(() => {}), 1500);
  } catch { /* no sound */ }
}

function confetti() {
  if (!celebrationEnabled("confetti")) return;
  try {
    if (window.matchMedia?.("(prefers-reduced-motion: reduce)").matches) return;
    const canvas = document.createElement("canvas");
    const dpr = window.devicePixelRatio || 1;
    const w = window.innerWidth, h = window.innerHeight;
    canvas.width = w * dpr;
    canvas.height = h * dpr;
    canvas.style.cssText = "position:fixed;inset:0;width:100%;height:100%;pointer-events:none;z-index:3000";
    document.body.appendChild(canvas);
    const g = canvas.getContext("2d");
    if (!g) { canvas.remove(); return; }
    g.scale(dpr, dpr);

    const color = theme.success;
    // Fired up and outward from low-centre, then pulled down by gravity.
    const bits = Array.from({ length: 70 }, () => {
      const angle = -Math.PI / 2 + (Math.random() - 0.5) * 1.5;
      const speed = 9 + Math.random() * 9;
      return {
        x: w / 2 + (Math.random() - 0.5) * 80,
        y: h * 0.7,
        vx: Math.cos(angle) * speed,
        vy: Math.sin(angle) * speed,
        size: 16 + Math.random() * 18,
        rot: Math.random() * Math.PI * 2,
        spin: (Math.random() - 0.5) * 0.3,
      };
    });

    const DURATION = 2600;
    const start = performance.now();
    const frame = (t: number) => {
      const age = t - start;
      g.clearRect(0, 0, w, h);
      const fade = age > DURATION - 700 ? Math.max(0, (DURATION - age) / 700) : 1;
      g.globalAlpha = fade;
      g.fillStyle = color;
      g.textAlign = "center";
      g.textBaseline = "middle";
      for (const b of bits) {
        b.vy += 0.32;      // gravity
        b.vx *= 0.99;      // a little air drag
        b.x += b.vx;
        b.y += b.vy;
        b.rot += b.spin;
        g.save();
        g.translate(b.x, b.y);
        g.rotate(b.rot);
        g.font = `700 ${b.size}px system-ui, sans-serif`;
        g.fillText("$", 0, 0);
        g.restore();
      }
      if (age < DURATION) requestAnimationFrame(frame);
      else canvas.remove();
    };
    requestAnimationFrame(frame);
  } catch { /* no confetti */ }
}

// celebratePayment plays the sound and the confetti together (or just one of them, for
// the preview in Settings). Each respects its own on/off switch.
export function celebratePayment(only?: CelebrationEffect) {
  if (only !== "confetti") playSound();
  if (only !== "sound") confetti();
}
