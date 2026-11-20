import { TriangleAlert } from "lucide-react";
import { useId, type ReactNode } from "react";

interface FieldProps {
  label: string;
  hint?: string;
  /** Shown under the field in the danger color, with an icon. */
  error?: string;
  /** Receives the ids to put on the control so that hint and error are read out with it. */
  children: (control: { id: string; describedBy: string | undefined; invalid: boolean }) => ReactNode;
}

/** A labelled form field: label above, hint and error below. */
export function Field({ label, hint, error, children }: FieldProps) {
  const id = useId();
  const hintId = `${id}-hint`;
  const errorId = `${id}-error`;
  const describedBy = [hint ? hintId : "", error ? errorId : ""].filter(Boolean).join(" ") || undefined;
  return (
    <div className="field">
      <label className="field__label" htmlFor={id}>
        {label}
      </label>
      {children({ id, describedBy, invalid: Boolean(error) })}
      {hint ? (
        <p className="field__hint" id={hintId}>
          {hint}
        </p>
      ) : null}
      {error ? (
        <p className="field__error" id={errorId}>
          <TriangleAlert size={16} aria-hidden="true" />
          {error}
        </p>
      ) : null}
    </div>
  );
}
