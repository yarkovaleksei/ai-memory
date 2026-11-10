import { vi } from "vitest";
import type { Entry, Item, ItemQuery, Plan, Project, Source } from "../api/bindings";

// Подмена модуля api/bindings для тестов: в jsdom нет Go-рантайма. Данные выдуманы
// и повторяют форму того, что отдаёт LibraryService на фикстурах.

const base = {
  sourceId: "claude:/fx/.claude",
  description: "",
  frontmatter: null,
  links: [],
  anchor: "",
  modTime: "2026-01-02T10:00:00Z",
  hash: "h",
  write: "direct",
  warnings: [],
} as const;

export const items: Item[] = [
  {
    ...base,
    id: "i-yarn",
    projectId: "path:/home/me/projects/demo",
    agent: "claude",
    kind: "claude-fact",
    type: "feedback",
    title: "Пакеты только через yarn",
    body: "Ставь пакеты через **yarn**. См. [[release-flow]] и [[missing-slug]].",
    file: "/fx/.claude/projects/-home-me-projects-demo/memory/prefer-yarn.md",
    frontmatter: { name: "prefer-yarn", type: "feedback" },
    links: [
      { text: "release-flow", targetId: "i-release" },
      { text: "missing-slug", targetId: "" },
    ],
  } as unknown as Item,
  {
    ...base,
    id: "i-release",
    projectId: "path:/home/me/projects/demo",
    agent: "codex",
    kind: "codex-rollout",
    type: "",
    title: "Релизный процесс",
    body: "Релиз — тег на main.",
    file: "/fx/.codex/memories_v2/rollout_summaries/demo_release_flow.md",
    warnings: [{ code: "claude.index.not-listed" }],
  } as unknown as Item,
  {
    ...base,
    id: "i-scratch",
    projectId: "path:/tmp/scratch-42",
    agent: "claude",
    kind: "claude-fact",
    type: "project",
    title: "Черновик эксперимента",
    body: "Временный каталог.",
    file: "/fx/.claude/projects/-tmp-scratch-42/memory/scratch-note.md",
  } as unknown as Item,
  {
    ...base,
    id: "i-note",
    sourceId: "codex:v2",
    projectId: "global",
    agent: "codex",
    kind: "codex-adhoc-note",
    type: "",
    title: "Сборка демо",
    body: "# Сборка демо\n\n- Собирается командой make demo.",
    file: "/fx/.codex/memories_v2/extensions/ad_hoc/notes/2026-01-03-sborka-demo.md",
    pending: true,
  } as unknown as Item,
  {
    ...base,
    id: "i-section",
    sourceId: "codex:v2",
    projectId: "global",
    agent: "codex",
    kind: "codex-summary-section",
    type: "",
    title: "User preferences",
    body: "- Prefers short answers.",
    file: "/fx/.codex/memories_v2/memory_summary.md",
    write: "via-note",
  } as unknown as Item,
];

export const projects: Project[] = [
  {
    id: "global",
    path: "",
    label: "",
    global: true,
    temp: false,
    resolved: true,
    agents: ["codex"],
    sourceIds: [],
    itemCount: 0,
  },
  {
    id: "path:/home/me/projects/demo",
    path: "/home/me/projects/demo",
    label: "demo",
    global: false,
    temp: false,
    resolved: true,
    agents: ["claude", "codex"],
    sourceIds: [],
    itemCount: 2,
  },
  {
    id: "path:/tmp/scratch-42",
    path: "/tmp/scratch-42",
    label: "scratch-42",
    global: false,
    temp: true,
    resolved: true,
    agents: ["claude"],
    sourceIds: [],
    itemCount: 1,
  },
] as unknown as Project[];

export const sources: Source[] = [
  {
    id: "claude:/fx/.claude",
    agent: "claude",
    label: ".claude",
    root: "/fx/.claude",
    readOnly: false,
    warnings: [],
  },
  {
    id: "codex:v2",
    agent: "codex",
    label: "Codex v2",
    root: "/fx/.codex/memories_v2",
    readOnly: false,
    warnings: [],
  },
] as unknown as Source[];

// Sources — vi.fn: тест «нет агента» подменяет список хранилищ.
export const listSources = vi.fn((): Promise<Source[]> => Promise.resolve(sources));

const temp = new Set(projects.filter((p) => p.temp).map((p) => p.id));

export const listItems = vi.fn((q: ItemQuery) => {
  let hits = items.filter((it) =>
    q.projectId ? it.projectId === q.projectId : q.includeTemp || !temp.has(it.projectId),
  );
  if (q.agent) hits = hits.filter((it) => it.agent === q.agent);
  if (q.q) {
    const needle = q.q.toLowerCase();
    hits = hits.filter((it) => (it.title + " " + it.body).toLowerCase().includes(needle));
  }
  return Promise.resolve({
    items: hits.map((it) => ({ ...it, body: "", snippet: q.q ? `…через \u0002${q.q}\u0003…` : "" })),
    total: hits.length,
  });
});

// Планы — то, что вернул бы PlansService: путь и содержимое до и после.
export const plan = (code: string, params: Record<string, string> = {}): Plan => ({
  id: `plan-${code}`,
  op: { code, params },
  changes: [
    {
      path: "/fx/.claude/projects/-home-me-projects-demo/memory/prefer-yarn.md",
      beforeExists: true,
      before: "старое\n",
      afterExists: true,
      after: "новое\n",
    },
  ],
});

const rejectCode = (code: string, params: Record<string, string> = {}) =>
  Promise.reject(Object.assign(new Error(code), { cause: { code, params } }));

export const entries: Entry[] = [
  {
    commit: "abc1234def",
    time: "2026-02-03T04:05:06Z",
    op: { code: "claude.fact.update", params: { name: "prefer-yarn" } },
    paths: ["/fx/.claude/projects/-home-me-projects-demo/memory/prefer-yarn.md"],
  },
];

export const plansService = {
  Status: vi.fn(() =>
    Promise.resolve({
      enabled: true,
      reason: "",
      codex: {
        generation: "v2",
        generationReason: "config",
        consolidation: {
          known: true,
          status: "done",
          finished: "2026-02-03T00:00:00Z",
          notBefore: "2999-01-01T00:00:00Z",
        },
        cooldownHours: 6,
      },
    }),
  ),
  Draft: vi.fn((id: string) =>
    id === "i-yarn"
      ? Promise.resolve({
          content: "---\nname: prefer-yarn\n---\n\nСтавь пакеты через yarn.\n",
          hash: "h-yarn",
          fact: {
            name: "prefer-yarn",
            description: "Только yarn",
            type: "feedback",
            title: "Пакеты только через yarn",
            hook: "",
            body: "Ставь пакеты через yarn.",
          },
        })
      : Promise.resolve({ content: "# Сборка демо\n", hash: "h-note", fact: null }),
  ),
  UpdateFact: vi.fn(() => Promise.resolve(plan("claude.fact.update", { name: "prefer-yarn" }))),
  UpdateSource: vi.fn(() => Promise.resolve(plan("codex.note.update", { file: "x.md" }))),
  CreateFact: vi.fn(() => Promise.resolve(plan("claude.fact.create", { name: "x" }))),
  DeleteFacts: vi.fn((ids: string[]) =>
    Promise.resolve(plan("claude.fact.delete", { count: String(ids.length) })),
  ),
  FixIndex: vi.fn(() => Promise.resolve(plan("claude.index.fix"))),
  CreateNote: vi.fn(() => Promise.resolve(plan("codex.note.create", { title: "x" }))),
  ForgetNote: vi.fn((title: string) => Promise.resolve(plan("codex.note.forget", { title }))),
  Merge: vi.fn((req: { target: string }) =>
    Promise.resolve(plan("merge", { count: "2", title: req.target })),
  ),
  TransferToClaude: vi.fn((_id: string, _src: string, _proj: string, fact: { name: string }) =>
    Promise.resolve(plan("transfer.copy-to-claude", { name: fact.name })),
  ),
  TransferToCodex: vi.fn((_id: string, title: string) =>
    Promise.resolve(plan("transfer.move-to-codex", { title })),
  ),
  Apply: vi.fn((): Promise<unknown> => Promise.resolve({ commit: "fff0000", stats: {} })),
  Discard: vi.fn(() => Promise.resolve()),
};

export const historyService = {
  List: vi.fn(() => Promise.resolve(entries)),
  Links: vi.fn((id: string) =>
    Promise.resolve(
      id === "i-yarn"
        ? [
            {
              direction: "to",
              move: false,
              itemId: "i-note",
              title: "Сборка демо",
              agent: "codex",
              time: "2026-02-03T04:05:06Z",
              commit: "c0ffee1",
            },
          ]
        : [],
    ),
  ),
  Revert: vi.fn((commit: string) => Promise.resolve(plan("revert", { commit, op: "claude.fact.update" }))),
};

export { rejectCode };

export const settingsService = {
  Get: vi.fn(() => Promise.resolve(settingsView(["~/.claude-personal"]))),
  Save: vi.fn((st: { extraClaudeProfiles: string[] }) =>
    Promise.resolve(settingsView(st.extraClaudeProfiles)),
  ),
  PickDirectory: vi.fn(() => Promise.resolve("/fx/home/.claude-work")),
};

function settingsView(extra: string[]) {
  return {
    settings: { extraClaudeProfiles: extra, codexHome: "", codexGeneration: "", showCodexInactive: false },
    defaultClaudeProfiles: ["/fx/home/.claude"],
    defaultCodexHome: "/fx/home/.codex",
    claudeProfiles: ["/fx/home/.claude", ...extra],
    codexHome: "/fx/home/.codex",
    codexGeneration: "v2",
    generationReason: "config",
    lockedClaudeProfiles: false,
    lockedCodexHome: false,
    configFile: "/fx/cfg/config.toml",
  };
}

// LLM: CLI агентов в тестах нет, ответы выдуманы.
export const llmService = {
  Agents: vi.fn((): Promise<string[]> => Promise.resolve(["claude", "codex"])),
  MergeDraft: vi.fn((ids: string[]) =>
    Promise.resolve({
      title: "Менеджер пакетов",
      description: "Чем ставить пакеты в демо",
      body: "Ставить пакеты только через yarn 4.\n",
      contradictions: [{ itemIds: ids.slice(0, 2), summary: "Разные версии yarn" }],
    }),
  ),
  Contradictions: vi.fn((ids: string[]) =>
    Promise.resolve([
      {
        summary: "Разные базовые ветки релиза",
        items: items.filter((it) => ids.includes(it.id)).slice(0, 2),
      },
    ]),
  ),
};

export const fakeBindings = () => ({
  SettingsService: settingsService,
  LLMService: llmService,
  Agent: { AgentClaude: "claude", AgentCodex: "codex" },
  PlansService: plansService,
  HistoryService: historyService,
  InfoService: {
    Info: () =>
      Promise.resolve({
        version: "9.9.9-test",
        configDir: "/fx/cfg",
        dataDir: "/fx/data",
        claudeProfiles: ["/fx/home/.claude", "/fx/home/.claude-work"],
        codexHome: "/fx/home/.codex",
      }),
  },
  LibraryService: {
    ListItems: listItems,
    GetItem: (id: string) => {
      const it = items.find((i) => i.id === id);
      return it
        ? Promise.resolve(it)
        : Promise.reject(Object.assign(new Error("item.not-found"), { cause: { code: "item.not-found" } }));
    },
    GetItems: (ids: string[]) => Promise.resolve(items.filter((i) => ids.includes(i.id))),
    Projects: () => Promise.resolve(projects),
    Sources: listSources,
    Rebuild: () => Promise.resolve({ sources: 1, projects: 3, items: 3, builtAt: "", duration: "5ms" }),
  },
  IndexUpdated: "index:updated",
  Events: { On: vi.fn(() => () => {}) },
  Browser: { OpenURL: vi.fn(() => Promise.resolve()) },
});
