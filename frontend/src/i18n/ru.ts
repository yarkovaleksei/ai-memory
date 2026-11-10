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
      history: "История",
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
      readonly: "Эта операция не поддерживается: запись идёт только через план с diff",
      item: {
        "not-found": "Запись не найдена — возможно, файл удалён",
      },
      library: {
        failed: "Не удалось прочитать память: %{detail}",
      },
      resource: {
        unknown: "Неизвестный ресурс %{resource}",
      },
      write: {
        "empty-plan": "План пуст — записывать нечего",
        "no-changes": "Изменений нет — записывать нечего",
        "path-invalid": "Недопустимый путь %{path}",
        "path-duplicate": "Файл %{path} встречается в плане дважды",
        symlink: "%{path} — символьная ссылка, запись через неё запрещена",
        "not-regular": "%{path} — не обычный файл",
        "outside-roots": "%{path} лежит вне каталогов памяти агентов",
        conflict: "Файл %{path} изменился с момента, как вы его открыли. Откройте запись заново",
        unreadable: "Не удалось прочитать %{path}: %{error}",
        failed: "Запись не удалась, файлы возвращены в прежнее состояние: %{error}",
        busy: "Файл %{path} занят другим процессом — повторите позже",
      },
      backup: {
        "git-missing": "Не найден git: без бэкапов запись выключена, просмотр работает",
        failed: "Ошибка репозитория бэкапов: %{error}",
        "record-failed": "Не удалось сохранить снимок в бэкап, ничего не записано: %{error}",
      },
      history: {
        "not-found": "Операция %{commit} не найдена в истории",
      },
      plan: {
        "not-found": "План устарел — постройте его заново",
        readonly: "«%{title}» нельзя править напрямую",
        "source-invalid": "Профиль Claude не найден",
        "delete-unsupported":
          "«%{title}» нельзя удалить: удаляются только факты Claude, для записей Codex есть «Забыть»",
      },
      claude: {
        fact: {
          "name-invalid": "Имя «%{name}»: строчные латинские буквы, цифры, «-» и «_», до 80 символов",
          exists: "Факт «%{name}» уже есть в этом проекте",
          "description-invalid": "Описание — одна непустая строка",
          "type-invalid": "Неизвестный тип %{type}",
          "body-empty": "Текст факта пуст",
          "index-line-invalid": "Заголовок и зацепка — одна строка, в заголовке нельзя «[» и «]»",
          "project-required": "Факт Claude принадлежит проекту — выберите проект",
        },
        frontmatter: {
          invalid: "Frontmatter не разобран (%{error}) — исправьте файл во вкладке «Исходник»",
        },
        "auto-memory-directory":
          "В settings.json профиля задан autoMemoryDirectory (%{dir}): в такие профили ai-memory пока не пишет",
      },
      codex: {
        "generated-readonly": "%{path} создаёт сам Codex — меняйте его через заметку",
        "consolidation-running": "Codex сейчас консолидирует память — повторите через пару минут",
        "lock-failed": "Не удалось взять блокировку памяти Codex: %{error}",
        note: {
          "delete-forbidden":
            "Файл заметки Codex удалять нельзя. Чтобы отменить заметку, создайте заметку «Забыть»",
          "title-invalid": "Заголовок заметки — одна непустая строка",
          "body-empty": "Текст заметки пуст",
        },
      },
    },
    ops: {
      "claude.fact.create": "Создать факт «%{name}»",
      "claude.fact.update": "Изменить факт «%{name}»",
      "claude.fact.delete": "Удалить факты Claude: %{count}",
      "claude.index.fix": "Исправить индекс MEMORY.md",
      "codex.note.create": "Новая заметка Codex «%{title}»",
      "codex.note.forget": "Заметка «Забыть»: %{title}",
      "codex.note.update": "Изменить заметку Codex %{file}",
      revert: "Откат: %{op}",
    },
    opKinds: {
      "claude.fact.create": "создание факта",
      "claude.fact.update": "правка факта",
      "claude.fact.delete": "удаление фактов",
      "claude.index.fix": "исправление индекса",
      "codex.note.create": "новая заметка",
      "codex.note.forget": "заметка «Забыть»",
      "codex.note.update": "правка заметки",
      revert: "откат",
    },
    plan: {
      intro:
        "Будет изменён %{smart_count} файл |||| Будут изменены %{smart_count} файла |||| Будут изменены %{smart_count} файлов",
      apply: "Записать",
      preview: "Показать изменения",
      applied: "Записано. Операцию можно откатить в истории",
      created: "новый",
      deleted: "удаляется",
      changed: "изменён",
    },
    actions: {
      edit: "Изменить",
      delete: "Удалить",
      fixIndex: "Исправить индекс",
      editNote: "Изменить заметку",
      refine: "Уточнить заметкой",
      forget: "Забыть",
      create: "Создать",
      createFact: "Факт Claude",
      createNote: "Заметка Codex",
      deleteSelected:
        "Удалить %{smart_count} факт |||| Удалить %{smart_count} факта |||| Удалить %{smart_count} фактов",
    },
    fact: {
      createTitle: "Новый факт Claude",
      editTitle: "Факт Claude",
      tabs: {
        form: "Поля",
        source: "Исходник",
      },
      brokenFrontmatter: "Frontmatter не разобран — исправьте файл целиком",
      fields: {
        project: "Проект",
        profile: "Профиль Claude",
        name: "Имя",
        type: "Тип",
        description: "Описание",
        title: "Заголовок в MEMORY.md",
        hook: "Зацепка в MEMORY.md",
        body: "Текст факта",
        raw: "Исходник файла",
      },
      help: {
        project: "Claude читает факты только в своём проекте",
        name: "Имя файла: латинские буквы, цифры, «-»; после создания не меняется",
        description: "Одна строка: по ней Claude решает, нужен ли факт",
        hook: "Пусто — возьмётся описание",
      },
    },
    note: {
      titles: {
        create: "Новая заметка Codex",
        refine: "Уточнить заметкой",
        forget: "Забыть",
        edit: "Заметка Codex",
      },
      fields: {
        title: "Заголовок",
        body: "Текст заметки",
        content: "Заметка целиком",
      },
      prefill: {
        refineTitle: "Уточнение: %{title}",
        forgetTitle: "Забыть: %{title}",
        forgetBody: "Исключи из памяти «%{title}»: это больше не верно.",
      },
      bodyHelp:
        "Codex читает заметку как данные, а не как поручение: пишите факт утверждением, по одному на пункт списка. Например: «- Пользователь ставит пакеты только через yarn 4», а не «Всегда используй yarn»",
      forgetHelp:
        "Запись Codex нельзя удалить напрямую. Эта заметка попросит Codex исключить факт при следующей консолидации — сформулируйте точно, что именно забыть",
    },
    codex: {
      pending: "Ожидает консолидации",
      notesOnly:
        "В память Codex пишутся только заметки, в активное поколение %{generation}. Сам Codex перепишет сводку, когда учтёт заметку",
      hint: {
        running: "Консолидация идёт прямо сейчас",
        after: "Codex учтёт заметку в первом ходе любой своей сессии после %{time}",
        next: "Codex учтёт заметку в первом же ходе любой своей сессии",
        unknown:
          "Codex учтёт заметку при следующей консолидации — в начале хода сессии, не чаще раза в %{hours} ч",
      },
    },
    history: {
      title: "История",
      subtitle: "Все записи ai-memory с бэкапом до и после. Откат — тоже операция с diff",
      loading: "Загрузка истории…",
      empty: "Операций пока не было",
      revert: "Откатить",
      columns: {
        time: "Когда",
        op: "Операция",
        files: "Файлы",
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
