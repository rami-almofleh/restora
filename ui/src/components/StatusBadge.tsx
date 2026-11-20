import { Circle, CircleAlert, CircleCheck, LoaderCircle, TriangleAlert } from "lucide-react";

type Tone = "success" | "warning" | "danger" | "progress" | "neutral";

const ICONS = {
  success: CircleCheck,
  warning: TriangleAlert,
  danger: CircleAlert,
  progress: LoaderCircle,
  neutral: Circle,
} as const;

interface StatusBadgeProps {
  tone: Tone;
  label: string;
}

/** Status is always shown as icon + text, never by color alone. */
export function StatusBadge({ tone, label }: StatusBadgeProps) {
  const Icon = ICONS[tone];
  return (
    <span className={`badge badge--${tone}`}>
      <Icon size={16} aria-hidden="true" className={tone === "progress" ? "spin" : undefined} />
      {label}
    </span>
  );
}
