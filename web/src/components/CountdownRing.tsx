// CountdownRing draws a circular TOTP countdown: a full ring at `total` seconds
// that depletes to empty at 0, with the seconds shown inside. Turns red in the
// last few seconds.
export function CountdownRing({ seconds, total = 30, size = 34 }: { seconds: number; total?: number; size?: number }) {
  const stroke = 3;
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  const frac = Math.max(0, Math.min(1, seconds / total));
  const offset = c * (1 - frac);
  const color = seconds <= 5 ? "var(--danger)" : "var(--accent)";
  const mid = size / 2;
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} className="totp-ring" style={{ direction: "ltr" }} aria-label={`${seconds}s`}>
      <circle cx={mid} cy={mid} r={r} fill="none" stroke="var(--border)" strokeWidth={stroke} />
      <circle
        cx={mid} cy={mid} r={r} fill="none" stroke={color} strokeWidth={stroke} strokeLinecap="round"
        strokeDasharray={c} strokeDashoffset={offset}
        transform={`rotate(-90 ${mid} ${mid})`}
      />
      <text x="50%" y="50%" textAnchor="middle" dominantBaseline="central" fontSize={size * 0.34} fill="var(--muted)">
        {seconds}
      </text>
    </svg>
  );
}
