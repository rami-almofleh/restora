import { NavLink } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { NAV_ITEMS } from "../../app/navigation";
import { Brand } from "./Brand";

export function Sidebar() {
  const { t } = useTranslation();
  return (
    <aside className="sidebar">
      <Brand />
      <nav aria-label={t("nav.main")}>
        <ul className="nav-list">
          {NAV_ITEMS.map((item) => (
            <li key={item.id}>
              <NavLink to={item.path} end={item.path === "/"} className="nav-link" title={t(item.labelKey)}>
                <item.icon size={20} aria-hidden="true" />
                <span className="nav-link__label">{t(item.labelKey)}</span>
              </NavLink>
            </li>
          ))}
        </ul>
      </nav>
    </aside>
  );
}
