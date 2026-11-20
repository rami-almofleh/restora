import { X } from "lucide-react";
import { useEffect, useId, useRef } from "react";
import { useTranslation } from "react-i18next";
import { NavLink } from "react-router-dom";
import type { NavItem } from "../../app/navigation";

interface MoreSheetProps {
  items: readonly NavItem[];
  onClose: () => void;
}

const FOCUSABLE = "a[href], button:not([disabled])";

/** Bottom sheet with the navigation entries that do not fit into the phone bottom bar. */
export function MoreSheet({ items, onClose }: MoreSheetProps) {
  const { t } = useTranslation();
  const titleId = useId();
  const sheetRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    sheetRef.current?.querySelector<HTMLElement>(FOCUSABLE)?.focus();
  }, []);

  const onKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape") {
      onClose();
      return;
    }
    if (event.key !== "Tab" || !sheetRef.current) {
      return;
    }
    // Keep keyboard focus inside the sheet while it is open.
    const focusable = Array.from(sheetRef.current.querySelectorAll<HTMLElement>(FOCUSABLE));
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last?.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first?.focus();
    }
  };

  return (
    <>
      <div className="sheet-scrim" onClick={onClose} aria-hidden="true" />
      <div
        className="sheet"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        ref={sheetRef}
        onKeyDown={onKeyDown}
      >
        <div className="sheet__header">
          <h2 className="sheet__title" id={titleId}>
            {t("nav.more")}
          </h2>
          <button type="button" className="icon-button" onClick={onClose} aria-label={t("nav.closeMenu")}>
            <X size={20} aria-hidden="true" />
          </button>
        </div>
        <ul className="nav-list">
          {items.map((item) => (
            <li key={item.id}>
              <NavLink to={item.path} className="nav-link" onClick={onClose}>
                <item.icon size={20} aria-hidden="true" />
                <span className="nav-link__label">{t(item.labelKey)}</span>
              </NavLink>
            </li>
          ))}
        </ul>
      </div>
    </>
  );
}
