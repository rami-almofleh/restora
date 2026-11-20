import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import type { Location } from "../../locations/api";
import { PolicyFormView } from "../../policies/components/PolicyFormView";
import { emptyForm } from "../../policies/form-model";
import { browserTimezone } from "../../policies/timezones";

interface PolicyStepProps {
  /** The storage location from step 1. The policy copies to it from the start. */
  location: Location;
  onSaved: () => void;
}

/** Step 2: the first policy. It starts with sensible values: all applications, every night, copied to the location. */
export function PolicyStep({ location, onSaved }: PolicyStepProps) {
  const { t } = useTranslation();
  const initial = useMemo(
    () => ({
      ...emptyForm(browserTimezone()),
      name: t("setup.policy.defaultName"),
      mode: "all" as const,
      scheduleType: "daily" as const,
      time: "02:00",
      exportTo: [location.id],
    }),
    [location.id, t],
  );

  return (
    <>
      <p className="setup-step__intro">{t("setup.policy.intro")}</p>
      <PolicyFormView initial={initial} embedded onSaved={onSaved} />
    </>
  );
}
