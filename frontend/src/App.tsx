import { Admin, CustomRoutes } from "react-admin";
import { Route } from "react-router-dom";
import { Dashboard } from "./Dashboard";
import { dataProvider } from "./dataProvider";
import { i18nProvider } from "./i18n";

// Ресурсов памяти пока нет (фаза 1). Без ресурсов и маршрутов react-admin показывает
// свой экран-заглушку, поэтому сведения о приложении доступны и как отдельная страница.
export const App = () => (
  <Admin dashboard={Dashboard} dataProvider={dataProvider} i18nProvider={i18nProvider} disableTelemetry>
    <CustomRoutes>
      <Route path="/about" element={<Dashboard />} />
    </CustomRoutes>
  </Admin>
);
