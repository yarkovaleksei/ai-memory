import polyglotI18nProvider from "ra-i18n-polyglot";
import ru from "./ru";

// Пока один язык. Английский словарь добавится рядом, переключатель — в настройках (ADR-008).
export const i18nProvider = polyglotI18nProvider(() => ru, "ru", [{ locale: "ru", name: "Русский" }], {
  allowMissing: false,
});
