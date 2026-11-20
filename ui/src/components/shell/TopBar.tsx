import { Brand } from "./Brand";
import { LanguageSelect } from "./LanguageSelect";
import { ThemeToggle } from "./ThemeToggle";

export function TopBar() {
  return (
    <header className="topbar">
      <div className="topbar__brand">
        <Brand />
      </div>
      <div className="topbar__controls">
        <LanguageSelect />
        <ThemeToggle />
      </div>
    </header>
  );
}
