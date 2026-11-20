import { Route, Routes } from "react-router-dom";
import { AppShell } from "../components/shell/AppShell";
import { ActivityPage } from "../features/activity/pages/ActivityPage";
import { LocationsPage } from "../features/locations/pages/LocationsPage";
import { EditLocationPage, NewLocationPage } from "../features/locations/pages/LocationFormPage";
import { PoliciesPage } from "../features/policies/pages/PoliciesPage";
import { EditPolicyPage, NewPolicyPage } from "../features/policies/pages/PolicyFormPage";
import { ApplicationDetailPage } from "../features/applications/pages/ApplicationDetailPage";
import { ApplicationsPage } from "../features/applications/pages/ApplicationsPage";
import { NotFoundPage } from "../features/placeholder/pages/NotFoundPage";
import { PlaceholderPage } from "../features/placeholder/pages/PlaceholderPage";
import { SettingsPage } from "../features/settings/pages/SettingsPage";
import { SetupPage } from "../features/setup/pages/SetupPage";
import { OverviewPage } from "../features/system/pages/OverviewPage";
import { NAV_ITEMS } from "./navigation";

export function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<OverviewPage />} />
        {NAV_ITEMS.filter((item) => item.placeholder).map((item) => (
          <Route key={item.id} path={item.path} element={<PlaceholderPage titleKey={item.labelKey} />} />
        ))}
        <Route path="/activity" element={<ActivityPage />} />
        <Route path="/applications" element={<ApplicationsPage />} />
        <Route path="/applications/:name" element={<ApplicationDetailPage />} />
        <Route path="/locations" element={<LocationsPage />} />
        <Route path="/locations/new" element={<NewLocationPage />} />
        <Route path="/locations/:id" element={<EditLocationPage />} />
        <Route path="/policies" element={<PoliciesPage />} />
        <Route path="/policies/new" element={<NewPolicyPage />} />
        <Route path="/policies/:id" element={<EditPolicyPage />} />
        <Route path="/setup" element={<SetupPage />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  );
}
