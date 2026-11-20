import type { ReactNode } from "react";

interface CheckboxProps {
  label: string;
  hint?: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  /** Extra content below the hint, for example a warning that shows while the box is checked. */
  children?: ReactNode;
}

/** A single option with a label and a short explanation. The whole row is the touch target. */
export function Checkbox({ label, hint, checked, onChange, children }: CheckboxProps) {
  return (
    <div className="checkbox">
      <label className="checkbox__row">
        <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
        <span>
          <span className="checkbox__label">{label}</span>
          {hint ? <span className="checkbox__hint">{hint}</span> : null}
        </span>
      </label>
      {children}
    </div>
  );
}
