import { useRef, useState } from "react";
import { FaEye, FaEyeSlash } from "react-icons/fa";

// PasswordInput — a masked input with a leading eye that briefly reveals the
// value (a "peek"), then re-hides it. Used by every password field.
export function PasswordInput({
  value,
  onChange,
  autoComplete,
  required,
  placeholder,
}: {
  value: string;
  onChange: (v: string) => void;
  autoComplete?: string;
  required?: boolean;
  placeholder?: string;
}) {
  const [show, setShow] = useState(false);
  const timer = useRef<number | undefined>(undefined);

  function peek() {
    setShow(true);
    clearTimeout(timer.current);
    timer.current = window.setTimeout(() => setShow(false), 700);
  }

  return (
    <span className="pw-input">
      <button type="button" className="pw-eye" title="Peek" tabIndex={-1} onClick={peek}>
        {show ? <FaEyeSlash /> : <FaEye />}
      </button>
      <input
        type={show ? "text" : "password"}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        autoComplete={autoComplete}
        required={required}
        placeholder={placeholder}
      />
    </span>
  );
}
