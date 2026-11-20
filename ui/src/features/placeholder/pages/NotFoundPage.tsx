import { SearchX } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Card } from "../../../components/Card";
import { PageHeader } from "../../../components/PageHeader";
import { StateMessage } from "../../../components/StateMessage";

export function NotFoundPage() {
  const { t } = useTranslation();
  return (
    <>
      <PageHeader title={t("notFound.title")} />
      <Card title={t("notFound.title")}>
        <StateMessage icon={SearchX} title={t("notFound.title")} text={t("notFound.text")}>
          <div className="state__actions">
            <Link className="button button--primary" to="/">
              {t("notFound.back")}
            </Link>
          </div>
        </StateMessage>
      </Card>
    </>
  );
}
