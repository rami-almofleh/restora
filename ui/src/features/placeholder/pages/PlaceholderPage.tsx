import { Hourglass } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Card } from "../../../components/Card";
import { PageHeader } from "../../../components/PageHeader";
import { StateMessage } from "../../../components/StateMessage";

interface PlaceholderPageProps {
  /** Translation key of the page title (the same key as in the navigation). */
  titleKey: string;
}

/** Stands in for a feature that is planned but not built yet. */
export function PlaceholderPage({ titleKey }: PlaceholderPageProps) {
  const { t } = useTranslation();
  return (
    <>
      <PageHeader title={t(titleKey)} />
      <Card title={t("placeholder.title")}>
        <StateMessage icon={Hourglass} title={t("placeholder.title")} text={t("placeholder.text")}>
          <div className="state__actions">
            <Link className="button button--secondary" to="/">
              {t("placeholder.back")}
            </Link>
          </div>
        </StateMessage>
      </Card>
    </>
  );
}
