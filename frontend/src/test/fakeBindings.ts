import { vi } from "vitest";
import type { Item, ItemQuery, Project, Source } from "../api/bindings";

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
] as unknown as Source[];

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

export const fakeBindings = () => ({
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
    Sources: () => Promise.resolve(sources),
    Rebuild: () => Promise.resolve({ sources: 1, projects: 3, items: 3, builtAt: "", duration: "5ms" }),
  },
  IndexUpdated: "index:updated",
  Events: { On: vi.fn(() => () => {}) },
  Browser: { OpenURL: vi.fn(() => Promise.resolve()) },
});
