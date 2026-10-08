// icons.tsx — dynamic, scalable icon resolver for react-icons.
//
// You can use ANY icon name from https://react-icons.github.io/react-icons/
// (e.g. "Bs0CircleFill", "FaPalette", "MdLock", "FaBowlFood", "IoFastFood"). We
// detect the pack(s) from the name's prefix and lazy-load only those on demand —
// so the whole ~40k-icon library is available without bundling it all up front.

import { useEffect, useState } from "react";
import type { IconType } from "react-icons";
import { FaRegQuestionCircle } from "react-icons/fa";

// Pack loaders keyed by react-icons module name. import("react-icons/xx") is a
// static string, so Vite code-splits each pack into its own lazily-loaded chunk.
const PACKS: Record<string, () => Promise<Record<string, unknown>>> = {
  ai: () => import("react-icons/ai"),
  bi: () => import("react-icons/bi"),
  bs: () => import("react-icons/bs"),
  cg: () => import("react-icons/cg"),
  ci: () => import("react-icons/ci"),
  di: () => import("react-icons/di"),
  fa: () => import("react-icons/fa"),
  fa6: () => import("react-icons/fa6"),
  fc: () => import("react-icons/fc"),
  fi: () => import("react-icons/fi"),
  gi: () => import("react-icons/gi"),
  go: () => import("react-icons/go"),
  gr: () => import("react-icons/gr"),
  hi: () => import("react-icons/hi"),
  hi2: () => import("react-icons/hi2"),
  im: () => import("react-icons/im"),
  io: () => import("react-icons/io"),
  io5: () => import("react-icons/io5"),
  lia: () => import("react-icons/lia"),
  lu: () => import("react-icons/lu"),
  md: () => import("react-icons/md"),
  pi: () => import("react-icons/pi"),
  ri: () => import("react-icons/ri"),
  rx: () => import("react-icons/rx"),
  si: () => import("react-icons/si"),
  sl: () => import("react-icons/sl"),
  tb: () => import("react-icons/tb"),
  tfi: () => import("react-icons/tfi"),
  ti: () => import("react-icons/ti"),
  vsc: () => import("react-icons/vsc"),
  wi: () => import("react-icons/wi"),
};

// Icon-name prefix → ordered list of packs to search. Crucially, Font Awesome 6,
// Ionicons 5, and Heroicons 2 icons keep the SAME name prefix as their older
// siblings (Fa*, Io*, Hi*) but live in different modules — so we must search both
// (newer-only icons like "FaBowlFood" / "IoFastFood" live in fa6 / io5, while the
// classic ones are in fa / io). We return the first pack that defines the name.
const PREFIX_PACKS: Record<string, string[]> = {
  Ai: ["ai"], Bi: ["bi"], Bs: ["bs"], Cg: ["cg"], Ci: ["ci"], Di: ["di"],
  Fa: ["fa", "fa6"], Fc: ["fc"], Fi: ["fi"], Gi: ["gi"], Go: ["go"], Gr: ["gr"],
  Hi: ["hi", "hi2"], Im: ["im"], Io: ["io5", "io"], Lia: ["lia"], Lu: ["lu"],
  Md: ["md"], Pi: ["pi"], Ri: ["ri"], Rx: ["rx"], Si: ["si"], Sl: ["sl"],
  Tb: ["tb"], Tfi: ["tfi"], Ti: ["ti"], Vsc: ["vsc"], Wi: ["wi"],
};

// Longest-first so "Tfi" wins over "Ti", "Lia" over any shorter match, etc.
const PREFIXES = Object.keys(PREFIX_PACKS).sort((a, b) => b.length - a.length);

// Cache each loaded pack (by module name) so we don't re-import it.
const packCache = new Map<string, Record<string, IconType>>();

// sanitizeIconName pulls a valid component name out of user input like
// "<Bs0CircleFill />" or " FaPalette " → "Bs0CircleFill" / "FaPalette".
export function sanitizeIconName(raw: string): string {
  const m = raw.match(/[A-Z][A-Za-z0-9]*/);
  return m ? m[0] : "";
}

function prefixOf(name: string): string | undefined {
  return PREFIXES.find((p) => name.startsWith(p));
}

// resolveIcon finds the icon component by name, searching each candidate pack for
// its prefix in order and loading them on demand. Returns null if not found.
async function resolveIcon(name: string): Promise<IconType | null> {
  const prefix = prefixOf(name);
  if (!prefix) return null;
  for (const pk of PREFIX_PACKS[prefix]) {
    let pack = packCache.get(pk);
    if (!pack) {
      try {
        pack = (await PACKS[pk]()) as Record<string, IconType>;
        packCache.set(pk, pack);
      } catch {
        continue;
      }
    }
    if (pack[name]) return pack[name];
  }
  return null;
}

// Icon renders a react-icons icon by name, loading its pack(s) on demand.
export function Icon({ name, size = 16 }: { name?: string; size?: number }) {
  const [Cmp, setCmp] = useState<IconType | null>(null);

  useEffect(() => {
    let alive = true;
    setCmp(null);
    if (!name) return;
    resolveIcon(name).then((c) => {
      if (alive) setCmp(() => c);
    });
    return () => {
      alive = false;
    };
  }, [name]);

  const C = Cmp ?? FaRegQuestionCircle; // fallback while loading / if unknown
  return <C size={size} />;
}
