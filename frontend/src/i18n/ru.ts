import russianMessages from "ra-language-russian";
import type { TranslationMessages } from "react-admin";

// Словарь интерфейса (ADR-008): встроенные сообщения react-admin из ra-language-russian
// и собственные ключи приложения в пространстве «aim». Каждая строка интерфейса — отсюда.
// Коды ошибок и предупреждений из Go содержат точки («claude.index.not-listed»):
// polyglot разворачивает вложенные объекты в такие же ключи через точку.
const ru: TranslationMessages = {
  ...russianMessages,
  resources: {
    items: {
      name: "Запись |||| Записи",
      fields: {
        q: "Поиск",
        title: "Название",
        agent: "Агент",
        type: "Тип",
        kind: "Вид",
        modTime: "Изменено",
        includeTemp: "С временными",
        warningsOnly: "С предупреждениями",
      },
    },
    projects: { name: "Проект |||| Проекты" },
    sources: { name: "Хранилище |||| Хранилища" },
  },
  aim: {
    app: {
      title: "ai-memory",
    },
    menu: {
      memory: "Память",
      about: "О программе",
    },
    agents: {
      claude: "Claude Code",
      codex: "Codex",
    },
    types: {
      user: "О пользователе",
      feedback: "Указания",
      project: "О проекте",
      reference: "Ссылки",
    },
    kinds: {
      "claude-fact": "Факт",
      "claude-index": "Индекс MEMORY.md",
      "codex-summary-section": "Раздел сводки",
      "codex-rollout": "Сводка сессии",
      "codex-adhoc-note": "Заметка",
      "codex-raw": "Сырые воспоминания",
    },
    write: {
      direct: "Правится напрямую",
      "via-note": "Правится через заметку",
      none: "Только чтение",
    },
    projects: {
      all: "Все проекты",
      global: "Глобальная память",
      unresolved: "Путь не восстановлен",
      temp: "Временные",
      count: "%{count} зап.",
      unresolvedHint: "Путь проекта не удалось восстановить, показано имя каталога памяти",
      loading: "Загрузка проектов…",
    },
    list: {
      rebuild: "Перечитать память",
      rebuilt: "Память перечитана: %{items} записей за %{duration}",
      empty: "Записей нет",
      emptyHint: "Выберите другой проект или измените фильтры",
    },
    preview: {
      empty: "Выберите запись слева",
      loading: "Загрузка записи…",
      tabs: {
        text: "Текст",
        source: "Исходник",
      },
      frontmatter: "Метаданные",
      body: "Текст",
      file: "Файл",
      anchor: "Раздел",
      source: "Хранилище",
      project: "Проект",
      warnings: "Предупреждения",
      noBody: "Текста нет",
      brokenLink: "Ссылка «%{text}» никуда не ведёт",
    },
    warnings: {
      unknown: "Предупреждение %{code}",
      claude: {
        settings: {
          "auto-memory-directory":
            "В settings.json задан autoMemoryDirectory — часть памяти может лежать в другом месте",
        },
        frontmatter: {
          missing: "Нет frontmatter: имя и тип взяты из имени файла",
          invalid: "Frontmatter не разобран: %{error}",
        },
        index: {
          "not-listed": "Файла нет в индексе MEMORY.md",
          "missing-file": "MEMORY.md ссылается на несуществующий файл %{file}",
        },
      },
      source: {
        "scan-failed": "Хранилище не прочитано: %{error}",
      },
    },
    errors: {
      unknown: "Неизвестная ошибка: %{detail}",
      readonly: "В этой версии память только для чтения",
      item: {
        "not-found": "Запись не найдена — возможно, файл удалён",
      },
      library: {
        failed: "Не удалось прочитать память: %{detail}",
      },
      resource: {
        unknown: "Неизвестный ресурс %{resource}",
      },
    },
    dashboard: {
      title: "О программе",
      subtitle: "Память Claude Code и Codex в одном окне",
      version: "Версия %{version}",
      loading: "Загрузка сведений о приложении…",
      error: "Не удалось получить сведения от приложения: %{message}",
      sections: {
        claude: "Профили Claude Code",
        codex: "Корень Codex",
        config: "Каталог настроек ai-memory",
        data: "Каталог данных ai-memory",
        sources: "Найденные хранилища",
      },
      noClaudeProfiles: "Профили не найдены",
      readOnlySource: "только чтение",
    },
  },
};

export default ru;
