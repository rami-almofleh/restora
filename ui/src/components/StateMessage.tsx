import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

interface StateMessageProps {
  icon: LucideIcon;
  title: string;
  text: string;
  tone?: "neutral" | "danger";
  /** Buttons, hints or technical details shown below the text. */
  children?: ReactNode;
}

/** One component for the empty and the error state of a page or card. */
export function StateMessage({ icon: Icon, title, text, tone = "neutral", children }: StateMessageProps) {
  return (
    <div className="state">
      <div className={tone === "danger" ? "state__icon state__icon--danger" : "state__icon"}>
        <Icon size={24} aria-hidden="true" />
      </div>
      <h3 className="state__title">{title}</h3>
      <p className="state__text">{text}</p>
      {children}
    </div>
  );
}
