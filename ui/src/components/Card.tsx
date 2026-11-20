import { useId, type ReactNode } from "react";

interface CardProps {
  title: string;
  /** Optional element shown at the end of the card header, for example a status badge. */
  aside?: ReactNode;
  /** Spans the full width of the card grid. */
  wide?: boolean;
  children: ReactNode;
}

export function Card({ title, aside, wide = false, children }: CardProps) {
  const titleId = useId();
  return (
    <section className={wide ? "card card--wide" : "card"} aria-labelledby={titleId}>
      <div className="card__header">
        <h2 className="card__title" id={titleId}>
          {title}
        </h2>
        {aside}
      </div>
      {children}
    </section>
  );
}
