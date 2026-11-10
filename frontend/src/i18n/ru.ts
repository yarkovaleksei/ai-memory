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
        kind: "Раздел",
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
      settings: "Настройки",
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
      instructions: "Инструкции",
      skill: "Навык",
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
      count: "%{smart_count} запись |||| %{smart_count} записи |||| %{smart_count} записей",
      search: "Найти проект",
      notFound: "Проекты не найдены",
      unresolvedHint: "Путь проекта не удалось восстановить, показано имя каталога памяти",
      loading: "Загрузка проектов…",
    },
    list: {
      rebuild: "Перечитать",
      rebuilt: "Память перечитана: %{items} записей за %{duration}",
      empty: "Записей нет",
      emptyHint: "Выберите другой проект или измените фильтры",
      selected:
        "Отмечена %{smart_count} запись |||| Отмечены %{smart_count} записи |||| Отмечено %{smart_count} записей",
      unselect: "Снять отметки",
    },
    preview: {
      empty: "Выберите запись",
      emptyHint: "Щёлкните запись в списке — здесь появятся её текст, метаданные и действия.",
      loading: "Загрузка записи…",
      tabs: {
        text: "Текст",
        source: "Исходник",
      },
      frontmatter: "Метаданные",
      body: "Текст",
      file: "Файл",
      anchor: "Раздел",
      target: "Симлинк на",
      repo: "Git-репозиторий",
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
          unlisted: "Факт %{file} есть на диске, но не попал в MEMORY.md",
        },
      },
      source: {
        "scan-failed": "Хранилище не прочитано: %{error}",
      },
      skill: {
        frontmatter: {
          missing: "Нет frontmatter: имя навыка взято из имени каталога",
          invalid: "Frontmatter навыка не разобран: %{error}",
        },
      },
      instructions: {
        frontmatter: {
          invalid: "Frontmatter не разобран: %{error}",
        },
      },
      codex: {
        skill: {
          "no-description": "Нет description: Codex такой навык не загрузит",
          "name-too-long": "name длиннее %{max} символов: Codex такой навык не загрузит",
          "file-symlink":
            "SKILL.md — симлинк: Codex такие файлы пропускает, подключайте симлинком весь каталог",
        },
        instructions: {
          shadowed: "Рядом лежит %{file}: Codex читает его вместо этого файла",
        },
      },
    },
    sections: {
      memory: "Память",
      instructions: "Инструкции",
      skill: "Навыки",
    },
    roles: {
      instructions: {
        main: "Основной файл",
        local: "Личный файл",
        override: "Замена AGENTS.md",
        rule: "Правило",
      },
      skill: {
        user: "Навык пользователя",
        project: "Навык проекта",
        system: "Системный",
        synced: "Из аккаунта claude.ai",
        memory: "Выведен из памяти",
      },
    },
    files: {
      shared: "Общий навык",
      sharedHelp: "Тот же каталог подключён в нескольких местах: правка затронет все",
      sharedWith: "Этот каталог навыка подключён и здесь:",
      inRepo: "В git-репозитории проекта",
      repoHelp: "Файл лежит в git-репозитории %{repo}: правка появится в его git status",
    },
    sources: {
      files: "%{label} — инструкции и навыки %{agent}",
      projectFiles: "Инструкции и навыки %{agent} в проектах",
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
        conflict: "Файл %{path} изменился с момента, как вы его открыли. Откройте запись заново.",
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
        "not-memory": "«%{title}» — не запись памяти: у инструкций и навыков свои операции",
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
      settings: {
        "generation-invalid": "Поколение Codex — v1, v2 или «определять автоматически»",
        "write-failed": "Не удалось записать настройки: %{error}",
        "dialog-failed": "Не удалось открыть выбор каталога: %{error}",
      },
      llm: {
        "not-found": "Не найдена команда %{agent} в PATH: установите CLI агента или выберите другого",
        failed: "%{agent} не ответил: %{detail}",
        timeout: "%{agent} не ответил за 5 минут — попробуйте меньше записей",
        canceled: "Запрос к модели остановлен",
        "bad-answer": "Ответ %{agent} не удалось разобрать: %{detail}",
        "agent-invalid": "Неизвестный агент: %{agent}",
        "too-large":
          "Записи не помещаются в один запрос: %{size} символов при пределе %{max} — отметьте меньше записей",
      },
      merge: {
        "too-few": "Для объединения выберите хотя бы две записи",
        "target-invalid": "Неизвестно, куда записать итог объединения",
      },
      section: {
        "not-instructions":
          "«%{title}» — не файл инструкций: разделы есть только у CLAUDE.md, AGENTS.md и правил",
        "not-found": "Раздела больше нет — файл изменился. Откройте диалог заново.",
        "same-file": "Раздел уже в этом файле — выберите другой",
        "target-invalid": "Неизвестно, куда вставить раздел — выберите файл из списка",
      },
      skill: {
        "not-skill": "«%{title}» — не навык",
        exists: "Навык уже есть в %{dir}",
        "target-invalid": "Неизвестно, куда ставить навык — выберите место из списка",
        "too-large":
          "Навык слишком велик для копии: больше %{files} файлов или %{bytes} байт. Поставьте его симлинком.",
      },
      unify: {
        already: "%{file} уже импортирует AGENTS.md",
        nothing: "У проекта нет ни CLAUDE.md, ни AGENTS.md",
        "nothing-to-move": "Отметьте хотя бы один блок: пустой AGENTS.md не нужен ни одному агенту",
        "project-required": "Каталог проекта неизвестен или не существует",
      },
      transfer: {
        "same-place": "Факт уже лежит в этом проекте и профиле — выберите другой или скопируйте",
        "same-agent": "Запись Codex переносится только в Claude Code",
      },
      codex: {
        "generated-readonly": "%{path} создаёт сам Codex — меняйте его через заметку",
        "consolidation-running": "Codex сейчас консолидирует память — повторите через пару минут",
        "lock-failed": "Не удалось взять блокировку памяти Codex: %{error}",
        note: {
          "delete-forbidden":
            "Файл заметки Codex удалять нельзя. Чтобы отменить заметку, создайте заметку «Забыть».",
          "title-invalid": "Заголовок заметки — одна непустая строка",
          "body-empty": "Текст заметки пуст",
        },
        files: {
          "delete-forbidden":
            "%{path} удалить нельзя: ai-memory удаляет файл инструкций или навыка Codex, только откатывая операцию, которая его создала",
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
      "transfer.copy-to-claude": "Копия в Claude Code: факт «%{name}»",
      "transfer.move-to-claude": "Перенос в Claude Code: факт «%{name}»",
      "transfer.copy-to-codex": "Копия в Codex: заметка «%{title}»",
      "transfer.move-to-codex": "Перенос в Codex: заметка «%{title}»",
      merge: "Объединение записей (%{count}) в «%{title}»",
      "instructions.update": "Изменить инструкции %{file}",
      "skill.update": "Изменить навык «%{name}»",
      "instructions.section-copy": "Копия правила «%{heading}» из %{from} в %{to}",
      "instructions.section-move": "Перенос правила «%{heading}» из %{from} в %{to}",
      "instructions.unify": "AGENTS.md — основной в проекте %{project}",
      "skill.install": "Навык «%{name}» в %{agent}",
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
      "transfer.copy-to-claude": "копия в Claude Code",
      "transfer.move-to-claude": "перенос в Claude Code",
      "transfer.copy-to-codex": "копия в Codex",
      "transfer.move-to-codex": "перенос в Codex",
      merge: "объединение",
      "instructions.update": "правка инструкций",
      "skill.update": "правка навыка",
      "instructions.section-copy": "копия правила",
      "instructions.section-move": "перенос правила",
      "instructions.unify": "AGENTS.md основным",
      "skill.install": "установка навыка",
      revert: "откат",
    },
    planWarnings: {
      plan: {
        "in-repo":
          "Файл лежит в git-репозитории %{repo}: правка появится в его git status. Коммитить её — ваше решение, ai-memory в чужие репозитории не коммитит.",
        "shared-skill":
          "Каталог навыка %{dir} подключён в нескольких местах: правка изменит навык у всех агентов и профилей, которые на него ссылаются.",
        "section-imports":
          "В разделе импорты Claude: %{imports}. Codex их не понимает и прочтёт как обычный текст — перенесите нужное содержимое сами.",
        "section-imported":
          "%{file} уже импортирует исходный файл (%{import}): Claude Code увидит правило дважды.",
        "section-exists": "В целевом файле уже есть раздел «%{heading}» — проверьте, не дубль ли это.",
        "target-shadowed":
          "Codex не читает %{file}: рядом лежит %{override}, и Codex берёт его вместо AGENTS.md.",
        "target-conditional":
          "%{file} — правило с paths: Claude Code загрузит его, только когда открыты подходящие файлы.",
        "codex-size":
          "Файл станет %{size} байт, а Codex читает инструкции проекта только до %{limit} байт: конец обрежется.",
        "skill-fields-ignored":
          "Поля frontmatter, которых %{agent} не понимает и молча проигнорирует: %{fields}.",
        "skill-no-description":
          "В SKILL.md нет description: Codex такой навык не загрузит. Допишите описание после установки.",
        "skill-name-long": "Имя «%{name}» длиннее 64 символов: Codex его не примет.",
        "skill-openai-yaml": "agents/openai.yaml — оформление навыка для Codex: Claude Code его не читает.",
        "skill-symlinks-skipped": "Симлинки внутри навыка не копируются: %{files}.",
        "skill-link":
          "Навык станет общим: оба агента будут читать каталог %{target}, и правка затронет обоих.",
        "claude-imports-agents":
          "Новый %{file} начинается с @AGENTS.md: пока CLAUDE.md нет, Claude Code читает AGENTS.md сам, а с CLAUDE.md перестанет — импорт сохраняет эти правила.",
      },
    },
    skill: {
      title: "Навык «%{title}» в %{agent}",
      target: "Куда",
      targets: {
        agents: "~/.agents/skills — общий каталог навыков, его читает Codex",
        "codex-home": "CODEX_HOME/skills — устаревший путь Codex",
        "project-agents": ".agents/skills проекта — навыки проекта для Codex",
        "project-claude": ".claude/skills проекта — навыки проекта для Claude Code",
        profile: "Профиль Claude %{label}",
      },
      recommended: "рекомендуется",
      exists: "навык с таким именем уже есть",
      mode: "Как поставить",
      copy: "Копией",
      link: "Симлинком",
      copyHelp: "Каталог навыка скопируется целиком: дальше копии живут независимо. Откат удалит копию.",
      linkHelp:
        "В цели появится симлинк на каталог навыка: оба агента будут читать один и тот же навык, и правка изменит его у обоих.",
      noTargets: "Ставить некуда: у второго агента нет каталога навыков этого уровня.",
    },
    unify: {
      action: "Сделать AGENTS.md основным",
      title: "AGENTS.md — основной, CLAUDE.md импортирует его",
      hint: {
        both: "CLAUDE.md и AGENTS.md проекта живут порознь. Общие правила можно держать в AGENTS.md — его читают оба агента, — а в CLAUDE.md оставить импорт @AGENTS.md.",
        "claude-only":
          "У проекта нет AGENTS.md: Codex не видит правил из CLAUDE.md. Общие правила можно перенести в AGENTS.md, а в CLAUDE.md оставить импорт.",
        "agents-only":
          "У проекта нет CLAUDE.md. Claude Code новых версий читает AGENTS.md и сам, но надёжнее CLAUDE.md с импортом @AGENTS.md.",
      },
      help: {
        both: "Отмеченные блоки CLAUDE.md уйдут в конец AGENTS.md. В CLAUDE.md останется остальное и строка %{import}.",
        "claude-only":
          "Отмеченные блоки CLAUDE.md станут новым AGENTS.md. В CLAUDE.md останется остальное и строка %{import}.",
        "agents-only": "Будет создан CLAUDE.md из одной строки %{import}.",
      },
      blocks: "Что перенести в AGENTS.md",
      override: "Рядом лежит AGENTS.override.md: пока он есть, Codex читает его, а не AGENTS.md.",
      empty: "Отметьте хотя бы один блок: пустой AGENTS.md не нужен ни одному агенту.",
    },
    section: {
      title: "Перенести правило из %{title}",
      section: "Раздел",
      preamble: "Текст до первого заголовка",
      text: "Текст раздела",
      target: "Куда",
      targetLabel: "%{agent} · %{title}",
      targetNew: "%{agent} · %{title} — будет создан",
      position: "Место",
      end: "В конец файла",
      after: "В конец раздела «%{heading}»",
      move: "Убрать из исходного файла",
      copyHelp: "Раздел скопируется, %{title} не изменится.",
      moveHelp: "Раздел уберётся из %{title} той же операцией. Откат вернёт оба файла.",
      imports: "В разделе импорты %{imports}: Codex их не понимает и прочтёт как текст.",
      noSections: "В файле нет текста — переносить нечего.",
      noTargets:
        "Вставить некуда: других файлов инструкций у проекта нет, и создать новый нельзя — каталог проекта неизвестен.",
    },
    file: {
      titles: {
        instructions: "Инструкции %{title}",
        skill: "Навык «%{title}»",
      },
      content: "Исходник файла",
    },
    plan: {
      intro:
        "Будет изменён %{smart_count} файл |||| Будут изменены %{smart_count} файла |||| Будут изменены %{smart_count} файлов",
      apply: "Записать",
      preview: "Показать изменения",
      applied: "Записано. Операцию можно откатить в истории.",
      confirm: "Понимаю, записать всё равно",
      link: "Симлинк на %{target}",
      unlink: "Удаляется симлинк на %{target}",
      created: "новый",
      deleted: "удаляется",
      changed: "изменён",
    },
    actions: {
      edit: "Изменить",
      copySection: "Перенести правило",
      installSkill: {
        claude: "Установить в Claude Code",
        codex: "Установить в Codex",
      },
      delete: "Удалить",
      fixIndex: "Исправить индекс",
      editNote: "Изменить заметку",
      refine: "Уточнить заметкой",
      forget: "Забыть",
      create: "Создать",
      createFact: "Факт Claude",
      createNote: "Заметка Codex",
      transfer: "Перенести",
      merge: "Объединить",
      contradictions: "Найти противоречия",
      deleteOnlyFacts: "Удаляются только факты Claude: для записей Codex есть «Забыть»",
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
        "Codex читает заметку как данные, а не как поручение: пишите факт утверждением, по одному на пункт списка. Например: «- Пользователь ставит пакеты только через yarn 4», а не «Всегда используй yarn».",
      forgetHelp:
        "Запись Codex нельзя удалить напрямую. Эта заметка попросит Codex исключить факт при следующей консолидации — сформулируйте точно, что именно забыть.",
    },
    codex: {
      pending: "Ожидает консолидации",
      cancelNote:
        "Пользователь отменил эту заметку: всё, что в ней было написано раньше, неверно и не должно оставаться в памяти.",
      notesOnly:
        "В память Codex пишутся только заметки, в активное поколение %{generation}. Сам Codex перепишет сводку, когда учтёт заметку.",
      hint: {
        running: "Консолидация идёт прямо сейчас",
        after: "Codex учтёт заметку в первом ходе любой своей сессии после %{time}",
        next: "Codex учтёт заметку в первом же ходе любой своей сессии",
        unknown:
          "Codex учтёт заметку при следующей консолидации — в начале хода сессии, не чаще раза в %{hours} ч",
      },
    },
    transfer: {
      title: "Перенос «%{title}»",
      target: "Куда",
      targets: {
        claude: "В Claude Code",
        codex: "В Codex",
      },
      move: "Убрать из источника",
      copyHelp: "Источник останется на месте. Пара «источник — копия» будет видна в обеих записях.",
      moveHelp: {
        claude: "Исходный факт Claude будет удалён тем же планом",
        codex:
          "Запись Codex нельзя удалить напрямую: тем же планом создастся заметка «Забыть», и Codex уберёт запись при консолидации",
      },
      already: "Эта запись уже переносилась сюда: «%{title}», %{time}",
      origin: "Источник: «%{title}» из памяти Claude Code, проект %{project}.",
      forgetBody:
        "Пользователь перенёс «%{title}» в память Claude Code: в памяти Codex этого больше быть не должно.",
    },
    settings: {
      title: "Настройки",
      loading: "Загрузка настроек…",
      intro:
        "Настройки хранятся в `%{file}`. Память агентов здесь не меняется; после сохранения ai-memory перечитает её без перезапуска.",
      default: "по умолчанию",
      saved: "Настройки сохранены, память перечитана",
      pickTitle: "Выберите каталог",
      pick: "Выбрать каталог",
      claude: {
        title: "Профили Claude Code",
        help: "Каталоги вроде `~/.claude` или того, что задаёт `CLAUDE_CONFIG_DIR`. Профили по умолчанию читаются всегда, здесь добавляются остальные.",
        locked: "Профили заданы переменной `AI_MEMORY_CLAUDE_DIRS` — список из файла сейчас не действует",
        manual: "Путь к профилю (можно с ~)",
        add: "Добавить",
        remove: "Убрать %{dir}",
      },
      codex: {
        title: "Codex",
        locked:
          "Корень Codex задан переменной `AI_MEMORY_CODEX_HOME` — значение из файла сейчас не действует",
        home: "Корень Codex",
        homeHelp: "Пусто — по умолчанию: `%{dir}`",
        generation: "Поколение памяти Codex",
        auto: "Определять автоматически (сейчас %{generation}: %{reason})",
        v1: "v1 — каталог `memories`",
        v2: "v2 — каталог `memories_v2`",
        showInactive: "Показывать неактивное поколение (только чтение)",
      },
      reasons: {
        settings: "задано здесь",
        config: "из `config.toml` Codex",
        activity: "по последней работе Codex",
        "only-one": "другого поколения нет",
        default: "умолчание Codex",
      },
    },
    contradictions: {
      title:
        "Противоречия среди %{smart_count} записи |||| Противоречия среди %{smart_count} записей |||| Противоречия среди %{smart_count} записей",
      help: "Модель получает только отмеченные записи и ищет те, что утверждают разное об одном и том же: разные ветки, версии, команды, правила. Ничего не записывается.",
      check: "Проверить через %{agent}",
      checking: "%{agent} сравнивает записи…",
      none: "Противоречий не найдено",
      close: "Закрыть",
    },
    merge: {
      title:
        "Объединить %{smart_count} запись |||| Объединить %{smart_count} записи |||| Объединить %{smart_count} записей",
      targets: {
        claude: "Итог — факт Claude",
        codex: "Итог — заметка Codex",
      },
      effect:
        "Тем же планом удаляются факты Claude: %{facts}; для записей Codex создаются заметки «Забыть»: %{notes}. Откатывается одной операцией.",
      forgetBody:
        "Пользователь объединил «%{title}» с похожими записями: эта формулировка больше не нужна, итог записан отдельно.",
    },
    llm: {
      agents: {
        claude: "Claude",
        codex: "Codex",
      },
      draft: "Черновик от %{agent}",
      drafting: "%{agent} сводит записи…",
      stop: "Остановить",
      draftHelp:
        "Модель сводит тексты записей в один черновик. Ничего не записывается, пока вы не проверите diff и не нажмёте «Записать».",
      drafted: "Черновик от %{agent} вставлен в форму — проверьте его",
      noAgents: "Команды `claude` и `codex` не найдены в `PATH` — запросы к модели недоступны",
      contradictions: "Записи расходятся — решите, что оставить в итоге:",
      between: "%{summary} (%{titles})",
    },
    links: {
      toCopy: "Скопировано в %{agent}: «%{title}»",
      toMove: "Перенесено в %{agent}: «%{title}»",
      fromCopy: "Копия записи %{agent} «%{title}»",
      fromMove: "Перенесено из %{agent}: «%{title}»",
    },
    history: {
      title: "История",
      subtitle: "Все записи ai-memory с бэкапом до и после. Откат — тоже операция с diff.",
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
