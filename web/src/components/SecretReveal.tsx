import { useState } from "react";
import { FaEye, FaEyeSlash } from "react-icons/fa";
import { revealPassword } from "../api";
import { errorAlert } from "../alerts";
import { CopyText } from "./CopyText";
import { IconButton } from "./IconButton";

// SecretReveal — the eye-toggle + hidden/revealed password used in the account
// list and the detail page. Reveals on demand, shows dots when hidden, and the
// revealed value is click-to-copy.
export function SecretReveal({ accountId }: { accountId: string }) {
  const [revealed, setRevealed] = useState<string | null>(null);

  async function toggle() {
    if (revealed) {
      setRevealed(null);
      return;
    }
    try {
      setRevealed(await revealPassword(accountId));
    } catch (err) {
      errorAlert((err as Error).message);
    }
  }

  return (
    <span className="secret">
      <IconButton title={revealed ? "Hide" : "Reveal"} onClick={toggle}>
        {revealed ? <FaEyeSlash /> : <FaEye />}
      </IconButton>
      {revealed ? <CopyText value={revealed} mono /> : <span className="muted">••••••••</span>}
    </span>
  );
}
