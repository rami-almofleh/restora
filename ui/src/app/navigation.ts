import {
  Activity,
  AppWindow,
  CalendarClock,
  HardDrive,
  LayoutDashboard,
  Settings,
  type LucideIcon,
} from "lucide-react";

export interface NavItem {
  id: string;
  path: string;
  icon: LucideIcon;
  /** Translation key of the label. */
  labelKey: string;
  /** Shorter label for the phone bottom bar, where each entry has little room. */
  shortLabelKey?: string;
  /** Shown directly in the phone bottom bar. All others go into the "More" sheet. */
  inBottomBar: boolean;
  /** Pages whose feature is not built yet show a placeholder. */
  placeholder: boolean;
}

export const NAV_ITEMS: readonly NavItem[] = [
  { id: "overview", path: "/", icon: LayoutDashboard, labelKey: "nav.overview", inBottomBar: true, placeholder: false },
  { id: "applications", path: "/applications", icon: AppWindow, labelKey: "nav.applications", shortLabelKey: "nav.applicationsShort", inBottomBar: true, placeholder: false },
  { id: "activity", path: "/activity", icon: Activity, labelKey: "nav.activity", inBottomBar: true, placeholder: false },
  { id: "policies", path: "/policies", icon: CalendarClock, labelKey: "nav.policies", inBottomBar: true, placeholder: false },
  { id: "locations", path: "/locations", icon: HardDrive, labelKey: "nav.locations", inBottomBar: false, placeholder: false },
  { id: "settings", path: "/settings", icon: Settings, labelKey: "nav.settings", inBottomBar: false, placeholder: false },
];
