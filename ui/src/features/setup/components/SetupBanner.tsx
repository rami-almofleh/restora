import { Rocket } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Card } from "../../../components/Card";
import { SETUP_STEPS, type SetupProgress } from "../progress";
import { writeFlag } from "../storage";

/** Shown on the overview while the setup is not finished. It names the next step and can be hidden for good. */
export function SetupBanner({ progress }: { progress: SetupProgress }) {
  const { t } = useTranslation();
  const [hidden, setHidden] = useState(false);
  if (progress.current === "done" || hidden) {
    return null;
  }

  return (
    <Card wide title={t("setup.banner.title")} aside={<Rocket size={20} aria-hidden="true" />}>
      <p>{t("setup.banner.text", { n: progress.index + 1, total: SETUP_STEPS.length, name: t(`setup.steps.${progress.current}`) })}</p>
      <div className="card__actions">
        <Link className="button button--primary" to="/setup">
          {progress.index === 0 ? t("setup.banner.start") : t("setup.banner.continue")}
        </Link>
        <button
          type="button"
          className="button button--secondary"
          onClick={() => {
            writeFlag("dismissed", true);
            setHidden(true);
          }}
        >
          {t("setup.banner.dismiss")}
        </button>
      </div>
    </Card>
  );
}
