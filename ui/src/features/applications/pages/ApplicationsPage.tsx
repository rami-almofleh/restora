import { AppWindow, RefreshCw, SearchX, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { PageHeader } from "../../../components/PageHeader";
import { SearchField } from "../../../components/SearchField";
import { SegmentedControl, type SegmentedOption } from "../../../components/SegmentedControl";
import { StateMessage } from "../../../components/StateMessage";
import { useDebouncedValue } from "../../../lib/use-debounced-value";
import { DeletedApplicationsCard } from "../../exports/components/DeletedApplicationsCard";
import type { VolumesFilter } from "../api";
import { ApplicationTable } from "../components/ApplicationTable";
import { useApplications } from "../use-applications";

const SEARCH_DELAY_MS = 250;

export function ApplicationsPage() {
  const { t } = useTranslation();
  const [search, setSearch] = useState("");
  const [volumes, setVolumes] = useState<VolumesFilter>("all");
  const [pages, setPages] = useState(1);
  const query = useDebouncedValue(search.trim(), SEARCH_DELAY_MS);
  const { state, reload } = useApplications(query, volumes, pages);

  const filterOptions: readonly SegmentedOption<VolumesFilter>[] = [
    { value: "all", label: t("applications.filter.all") },
    { value: "with", label: t("applications.filter.with") },
    { value: "without", label: t("applications.filter.without") },
  ];
  const filtered = search !== "" || volumes !== "all";

  const changeSearch = (value: string) => {
    setSearch(value);
    setPages(1);
  };
  const changeVolumes = (value: VolumesFilter) => {
    setVolumes(value);
    setPages(1);
  };
  const resetFilters = () => {
    changeSearch("");
    changeVolumes("all");
  };

  return (
    <>
      <PageHeader title={t("applications.title")} subtitle={t("applications.subtitle")} />

      <div className="toolbar">
        <SearchField
          label={t("applications.search.label")}
          placeholder={t("applications.search.placeholder")}
          clearLabel={t("applications.search.clear")}
          value={search}
          onChange={changeSearch}
        />
        <SegmentedControl
          label={t("applications.filter.label")}
          name="applications-volumes"
          value={volumes}
          options={filterOptions}
          onChange={changeVolumes}
        />
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
        <StateMessage
          tone="danger"
          icon={TriangleAlert}
          title={t("applications.error.title")}
          text={`${t("applications.error.text")} ${t("applications.error.hint")}`}
        >
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
          <StateMessage icon={SearchX} title={t("applications.noResults.title")} text={t("applications.noResults.text")}>
            <div className="state__actions">
              <button type="button" className="button button--secondary" onClick={resetFilters}>
                {t("applications.noResults.reset")}
              </button>
            </div>
          </StateMessage>
        ) : (
          <StateMessage icon={AppWindow} title={t("applications.empty.title")} text={t("applications.empty.text")}>
            <div className="state__actions">
              <button type="button" className="button button--primary" onClick={reload}>
                <RefreshCw size={18} aria-hidden="true" />
                {t("applications.empty.refresh")}
              </button>
            </div>
          </StateMessage>
        )
      ) : null}

      {state.status === "ready" && state.data.items.length > 0 ? (
        <>
          <p className="result-count" aria-live="polite">
            {t("applications.count", { total: state.data.total })}
          </p>
          <ApplicationTable items={state.data.items} />
          {state.data.hasMore ? (
            <div className="load-more">
              <button type="button" className="button button--secondary" onClick={() => setPages((n) => n + 1)}>
                {t("common.loadMore")}
              </button>
            </div>
          ) : null}
        </>
      ) : null}

      <DeletedApplicationsCard />
    </>
  );
}
