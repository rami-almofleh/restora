import { PlugZap, Plus, Settings2 } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Card } from "../../../components/Card";
import type { Location } from "../../locations/api";
import { requestLocationTest } from "../../locations/api";
import { LocationFormView } from "../../locations/components/LocationFormView";
import { TestBadge } from "../../locations/components/TestBadge";
import { TestResult } from "../../locations/components/TestResult";
import { emptyForm, formFromLocation } from "../../locations/form-model";

interface LocationStepProps {
  /** All storage locations, newest first. None of them works yet, or this step would be done. */
  locations: Location[];
  onChanged: () => void;
}

/** Step 1: where backups are kept outside the cluster. Saving tests the connection; the step ends when it works. */
export function LocationStep({ locations, onChanged }: LocationStepProps) {
  const { t } = useTranslation();
  const newest = locations[0];
  const [mode, setMode] = useState<"auto" | "new" | "edit">("auto");
  const [retesting, setRetesting] = useState(false);

  const saved = () => {
    setMode("auto");
    onChanged();
  };

  if (!newest || mode === "new") {
    return (
      <>
        <p className="setup-step__intro">{t("setup.location.intro")}</p>
        <LocationFormView initial={emptyForm()} embedded onSaved={saved} onCancel={newest ? () => setMode("auto") : undefined} />
      </>
    );
  }
  if (mode === "edit") {
    return (
      <LocationFormView
        id={newest.id}
        credentialsSet={newest.s3?.credentialsSet ?? false}
        initial={formFromLocation(newest)}
        embedded
        onSaved={saved}
        onCancel={() => setMode("auto")}
      />
    );
  }

  const retest = async () => {
    setRetesting(true);
    try {
      await requestLocationTest(newest.id);
    } finally {
      setRetesting(false);
      onChanged();
    }
  };

  return (
    <Card title={newest.name} aside={<TestBadge state={newest.test.state} />}>
      <TestResult test={newest.test} />
      <div className="card__actions">
        <button type="button" className="button button--secondary" onClick={() => void retest()} disabled={retesting || newest.test.state === "pending"}>
          <PlugZap size={18} aria-hidden="true" />
          {t("locations.card.test")}
        </button>
        <button type="button" className="button button--secondary" onClick={() => setMode("edit")}>
          <Settings2 size={18} aria-hidden="true" />
          {t("setup.location.change")}
        </button>
        <button type="button" className="button button--secondary" onClick={() => setMode("new")}>
          <Plus size={18} aria-hidden="true" />
          {t("setup.location.another")}
        </button>
      </div>
    </Card>
  );
}
