import InfoIcon from "@mui/icons-material/InfoOutlined";
import { useEffect, type ReactNode } from "react";
import { Layout as RaLayout, Menu as RaMenu, useRefresh, useTranslate } from "react-admin";
import { Events, IndexUpdated } from "../api/bindings";

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
      <RaMenu.Item to="/about" primaryText={translate("aim.menu.about")} leftIcon={<InfoIcon />} />
    </RaMenu>
  );
};

export const Layout = ({ children }: { children: ReactNode }) => {
  useIndexUpdates();
  return <RaLayout menu={Menu}>{children}</RaLayout>;
};
