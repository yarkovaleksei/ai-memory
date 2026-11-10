import SimilarIcon from "@mui/icons-material/CompareArrows";
import HistoryIcon from "@mui/icons-material/History";
import InfoIcon from "@mui/icons-material/InfoOutlined";
import SettingsIcon from "@mui/icons-material/SettingsOutlined";
import { useEffect, type ReactNode } from "react";
import { Layout as RaLayout, Menu as RaMenu, useRefresh, useTranslate } from "react-admin";
import { Events, IndexUpdated } from "../api/bindings";
import { PlanProvider } from "../edit/plans";

// useIndexUpdates перезапрашивает данные, когда Go пересобрал индекс после
// изменения файлов памяти: окно показывает свежую память без кнопки «обновить».
export const useIndexUpdates = () => {
  const refresh = useRefresh();
  useEffect(() => Events.On(IndexUpdated, () => refresh()), [refresh]);
};

// Ресурсы projects и sources служебные (дерево и подписи), в меню их нет.
const Menu = () => {
  const translate = useTranslate();
  return (
    <RaMenu>
      <RaMenu.ResourceItem name="items" />
      <RaMenu.Item to="/similar" primaryText={translate("aim.menu.similar")} leftIcon={<SimilarIcon />} />
      <RaMenu.Item to="/history" primaryText={translate("aim.menu.history")} leftIcon={<HistoryIcon />} />
      <RaMenu.Item to="/settings" primaryText={translate("aim.menu.settings")} leftIcon={<SettingsIcon />} />
      <RaMenu.Item to="/about" primaryText={translate("aim.menu.about")} leftIcon={<InfoIcon />} />
    </RaMenu>
  );
};

export const Layout = ({ children }: { children: ReactNode }) => {
  useIndexUpdates();
  // Диалог плана — над любым экраном: правка из просмотра, откат из истории.
  return (
    <RaLayout menu={Menu}>
      <PlanProvider>{children}</PlanProvider>
    </RaLayout>
  );
};
