import type { ReactNode } from "react";

// Modal — the single backdrop + centered panel used by every dialog. Backdrop
// clicks are intentionally ignored so an accidental click-away can't discard
// in-progress form input; dialogs are dismissed via their Cancel/close button.
// onClose is kept in the signature for those buttons to call.
export function Modal({
  wide = false,
  children,
}: {
  onClose?: () => void;
  wide?: boolean;
  children: ReactNode;
}) {
  return (
    <div className="modal-backdrop">
      <div className={`modal ${wide ? "wide" : ""}`.trim()}>
        {children}
      </div>
    </div>
  );
}
