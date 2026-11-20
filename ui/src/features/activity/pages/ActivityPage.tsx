import { Activity, AppWindow, RefreshCw, SearchX, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Card } from "../../../components/Card";
import { PageHeader } from "../../../components/PageHeader";
import { SearchField } from "../../../components/SearchField";
import { SegmentedControl, type SegmentedOption } from "../../../components/SegmentedControl";
import { StateMessage } from "../../../components/StateMessage";
import { useDebouncedValue } from "../../../lib/use-debounced-value";
import type { StatusFilter, TypeFilter } from "../api";
import { ActivityItem } from "../components/ActivityItem";
import { useActivity } from "../use-activity";

const SEARCH_DELAY_MS = 250;

export function ActivityPage() {
  const { t } = useTranslation();
  const [search, setSearch] = useState("");
  const [type, setType] = useState<TypeFilter>("all");
  const [status, setStatus] = useState<StatusFilter>("all");
  const [pages, setPages] = useState(1);
  const query = useDebouncedValue(search.trim(), SEARCH_DELAY_MS);
  const { state, reload } = useActivity({ query, type, status }, pages);

  const counts = state.status === "ready" ? state.data.meta.counts : undefined;
  const statusOption = (value: StatusFilter, count?: number): SegmentedOption<StatusFilter> => ({
    value,
    label: count === undefined ? t(`activity.status.${value}`) : t("activity.status.withCount", { label: t(`activity.status.${value}`), n: count }),
  });
  const statusOptions: readonly SegmentedOption<StatusFilter>[] = [
    statusOption("all", counts?.total),
    statusOption("active", counts?.active),
    statusOption("succeeded", counts?.succeeded),
    statusOption("failed", counts?.failed),
  ];
  const typeOptions: readonly SegmentedOption<TypeFilter>[] = [
    { value: "all", label: t("activity.typeFilter.all") },
    { value: "backup", label: t("activity.typeFilter.backup") },
    { value: "restore", label: t("activity.typeFilter.restore") },
  ];
  const filtered = search !== "" || type !== "all" || status !== "all";

  // Every filter change starts again at the first page.
  const changeSearch = (value: string) => {
    setSearch(value);
    setPages(1);
  };
  const changeType = (value: TypeFilter) => {
    setType(value);
    setPages(1);
  };
  const changeStatus = (value: StatusFilter) => {
    setStatus(value);
    setPages(1);
  };
  const resetFilters = () => {
    changeSearch("");
    changeType("all");
    changeStatus("all");
  };

  return (
    <>
      <PageHeader title={t("activity.title")} subtitle={t("activity.subtitle")} />

      <div className="toolbar">
        <SearchField
          label={t("activity.search.label")}
          placeholder={t("activity.search.placeholder")}
          clearLabel={t("activity.search.clear")}
          value={search}
          onChange={changeSearch}
        />
        <SegmentedControl label={t("activity.typeFilter.label")} name="activity-type" value={type} options={typeOptions} onChange={changeType} />
        <SegmentedControl label={t("activity.status.label")} name="activity-status" value={status} options={statusOptions} onChange={changeStatus} />
      </div>

      {state.status === "loading" ? (
        <div className="skeleton-stack" aria-busy="true">
          <span className="sr-only">{t("common.loading")}</span>
          <div className="skeleton skeleton--row" />
          <div className="skeleton skeleton--row" />
          <div className="skeleton skeleton--row" />
        </div>
      ) : null}

      {state.status === "error" ? (
        <StateMessage tone="danger" icon={TriangleAlert} title={t("activity.error.title")} text={`${t("activity.error.text")} ${t("activity.error.hint")}`}>
          <div className="state__actions">
            <button type="button" className="button button--secondary" onClick={reload}>
              <RefreshCw size={18} aria-hidden="true" />
              {t("common.retry")}
            </button>
          </div>
          <details className="details">
            <summary>{t("common.technicalDetails")}</summary>
            <code>
              {state.error.code}: {state.error.message}
            </code>
          </details>
        </StateMessage>
      ) : null}

      {state.status === "ready" && state.data.items.length === 0 ? (
        filtered ? (
          <StateMessage icon={SearchX} title={t("activity.noResults.title")} text={t("activity.noResults.text")}>
            <div className="state__actions">
              <button type="button" className="button button--secondary" onClick={resetFilters}>
                {t("activity.noResults.reset")}
              </button>
            </div>
          </StateMessage>
        ) : (
          <StateMessage icon={Activity} title={t("activity.empty.title")} text={t("activity.empty.text")}>
            <div className="state__actions">
              <Link className="button button--primary" to="/applications">
                <AppWindow size={18} aria-hidden="true" />
                {t("activity.empty.action")}
              </Link>
            </div>
          </StateMessage>
        )
      ) : null}

      {state.status === "ready" && state.data.items.length > 0 ? (
        <>
          <p className="result-count" aria-live="polite">
            {t("activity.count", { total: state.data.total })}
          </p>
          <Card wide title={t("activity.list.title")}>
            <ul className="job-list">
              {state.data.items.map((entry) => (
                <ActivityItem key={entry.id} entry={entry} />
              ))}
            </ul>
          </Card>
          {state.data.hasMore ? (
            <div className="load-more">
              <button type="button" className="button button--secondary" onClick={() => setPages((n) => n + 1)}>
                {t("common.loadMore")}
              </button>
            </div>
          ) : null}
        </>
      ) : null}
    </>
  );
}
