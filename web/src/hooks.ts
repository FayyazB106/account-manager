import { useEffect, useState } from "react";

// useMediaQuery tracks whether a CSS media query currently matches, re-rendering
// on change. Used to adapt layouts to viewport size (e.g. mobile vs desktop).
export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(() =>
    typeof window !== "undefined" && window.matchMedia(query).matches);
  useEffect(() => {
    const mq = window.matchMedia(query);
    const onChange = () => setMatches(mq.matches);
    onChange();
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, [query]);
  return matches;
}
