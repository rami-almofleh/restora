import { X } from "lucide-react";
import { useEffect, useId, useRef, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

interface DialogProps {
  title: string;
  onClose: () => void;
  children: ReactNode;
  /** Buttons. Put the safe choice (cancel) first: it gets the focus when the dialog opens. */
  footer: ReactNode;
}

/**
 * Modal dialog: a bottom sheet on phones, centered on larger screens.
 * It uses the browser's own dialog element, which traps focus, closes on Escape and dims the page.
 * Render it only while it should be open.
 */
export function Dialog({ title, onClose, children, footer }: DialogProps) {
  const { t } = useTranslation();
  const titleId = useId();
  const ref = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const dialog = ref.current;
    if (dialog && !dialog.open) {
      dialog.showModal();
    }
  }, []);

  return (
    <dialog
      ref={ref}
      className="dialog"
      aria-labelledby={titleId}
      onClose={onClose}
      onClick={(event) => {
        // A click on the dimmed backdrop lands on the dialog element itself.
        if (event.target === ref.current) {
          onClose();
        }
      }}
    >
      <div className="dialog__header">
        <h2 className="dialog__title" id={titleId}>
          {title}
        </h2>
        <button type="button" className="icon-button" onClick={onClose} aria-label={t("common.close")}>
          <X size={20} aria-hidden="true" />
        </button>
      </div>
      <div className="dialog__body">{children}</div>
      <div className="dialog__footer">{footer}</div>
    </dialog>
  );
}
