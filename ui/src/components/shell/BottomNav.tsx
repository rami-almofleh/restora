import { Ellipsis } from "lucide-react";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { NavLink, useLocation } from "react-router-dom";
import { NAV_ITEMS } from "../../app/navigation";
import { MoreSheet } from "./MoreSheet";

const BAR_ITEMS = NAV_ITEMS.filter((item) => item.inBottomBar);
const MORE_ITEMS = NAV_ITEMS.filter((item) => !item.inBottomBar);

export function BottomNav() {
  const { t } = useTranslation();
  const [moreOpen, setMoreOpen] = useState(false);
  const moreButtonRef = useRef<HTMLButtonElement>(null);
  const { pathname } = useLocation();
  const moreActive = MORE_ITEMS.some((item) => pathname.startsWith(item.path));

  const closeMore = () => {
    setMoreOpen(false);
    moreButtonRef.current?.focus();
  };

  return (
    <>
      <nav className="bottom-nav" aria-label={t("nav.main")}>
        {BAR_ITEMS.map((item) => (
          <NavLink key={item.id} to={item.path} end={item.path === "/"} className="bottom-nav__item">
            <item.icon size={22} aria-hidden="true" />
            <span>{t(item.shortLabelKey ?? item.labelKey)}</span>
          </NavLink>
        ))}
        <button
          type="button"
          className="bottom-nav__item"
          ref={moreButtonRef}
          aria-haspopup="dialog"
          aria-expanded={moreOpen}
          aria-current={moreActive ? "page" : undefined}
          onClick={() => setMoreOpen(true)}
        >
          <Ellipsis size={22} aria-hidden="true" />
          <span>{t("nav.more")}</span>
        </button>
      </nav>
      {moreOpen ? <MoreSheet items={MORE_ITEMS} onClose={closeMore} /> : null}
    </>
  );
}
