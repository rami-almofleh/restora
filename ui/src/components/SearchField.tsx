import { Search, X } from "lucide-react";

interface SearchFieldProps {
  /** Accessible name of the field. */
  label: string;
  placeholder: string;
  clearLabel: string;
  value: string;
  onChange: (value: string) => void;
}

export function SearchField({ label, placeholder, clearLabel, value, onChange }: SearchFieldProps) {
  return (
    <div className="search-field">
      <Search className="search-field__icon" size={20} aria-hidden="true" />
      <input
        className="search-field__input"
        type="search"
        aria-label={label}
        placeholder={placeholder}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
      {value ? (
        <button type="button" className="icon-button search-field__clear" aria-label={clearLabel} onClick={() => onChange("")}>
          <X size={20} aria-hidden="true" />
        </button>
      ) : null}
    </div>
  );
}
