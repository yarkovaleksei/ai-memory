import MemoryIcon from "@mui/icons-material/Psychology";
import { Admin, CustomRoutes, Resource } from "react-admin";
import { Route } from "react-router-dom";
import { Dashboard } from "./Dashboard";
import { dataProvider } from "./dataProvider";
import { HistoryPage } from "./history/HistoryPage";
import { SettingsPage } from "./settings/SettingsPage";
import { i18nProvider } from "./i18n";
import { ItemList } from "./items/ItemList";
import { Layout } from "./layout/Layout";
import { darkTheme, lightTheme } from "./theme";

// Первый ресурс — главный экран. Светлая и тёмная темы — штатные с правками
// (theme.ts): переключатель
// появляется в шапке сам, выбор react-admin запоминает в своём store.
export const App = () => (
  <Admin
    dataProvider={dataProvider}
    i18nProvider={i18nProvider}
    layout={Layout}
    theme={lightTheme}
    darkTheme={darkTheme}
    disableTelemetry
  >
    <Resource name="items" list={ItemList} icon={MemoryIcon} options={{ label: "aim.menu.memory" }} />
    <Resource name="projects" />
    <Resource name="sources" />
    <CustomRoutes>
      <Route path="/history" element={<HistoryPage />} />
      <Route path="/settings" element={<SettingsPage />} />
      <Route path="/about" element={<Dashboard />} />
    </CustomRoutes>
  </Admin>
);
