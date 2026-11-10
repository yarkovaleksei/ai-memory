import russianMessages from "ra-language-russian";
import type { TranslationMessages } from "react-admin";

// Словарь интерфейса (ADR-008): встроенные сообщения react-admin из ra-language-russian
// и собственные ключи приложения в пространстве «aim». Каждая строка интерфейса — отсюда.
const ru: TranslationMessages = {
  ...russianMessages,
  aim: {
    app: {
      title: "ai-memory",
    },
    dashboard: {
      title: "ai-memory",
      subtitle: "Память Claude Code и Codex в одном окне",
      version: "Версия %{version}",
      loading: "Загрузка сведений о приложении…",
      error: "Не удалось получить сведения от приложения: %{message}",
      sections: {
        claude: "Профили Claude Code",
        codex: "Корень Codex",
        config: "Каталог настроек ai-memory",
        data: "Каталог данных ai-memory",
      },
      noClaudeProfiles: "Профили не найдены",
    },
  },
};

export default ru;
