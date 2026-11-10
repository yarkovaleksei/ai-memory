import { describe, expect, it } from "vitest";
import { AppError, toAppError } from "./api/errors";
import { RELEVANCE, toItemQuery } from "./dataProvider";
import { linkify } from "./items/Markdown";
import { splitSnippet } from "./items/text";

describe("toItemQuery", () => {
  it("переводит параметры react-admin в запрос Go", () => {
    const q = toItemQuery({
      pagination: { page: 3, perPage: 25 },
      sort: { field: "title", order: "ASC" },
      filter: { q: "ветк", agent: "codex", includeTemp: true, projectId: "global" },
    });
    expect(q).toMatchObject({
      q: "ветк",
      agent: "codex",
      projectId: "global",
      includeTemp: true,
      warningsOnly: false,
      sort: "title",
      desc: false,
      offset: 50,
      limit: 25,
    });
  });

  it("релевантность и неизвестные поля — сортировка по умолчанию в Go", () => {
    expect(toItemQuery({ sort: { field: RELEVANCE, order: "DESC" } }).sort).toBe("");
    expect(toItemQuery({ sort: { field: "file; DROP", order: "DESC" } }).sort).toBe("");
  });
});

describe("toAppError", () => {
  it("берёт код и параметры из cause ошибки Wails", () => {
    const err = toAppError(
      Object.assign(new Error("x"), { cause: { code: "library.failed", params: { detail: "d" } } }),
    );
    expect(err).toBeInstanceOf(AppError);
    expect(err.message).toBe("aim.errors.library.failed");
    expect(err.params).toEqual({ detail: "d" });
  });

  it("неизвестная ошибка сохраняет текст в detail", () => {
    expect(toAppError(new Error("boom")).params).toEqual({ detail: "boom" });
  });
});

describe("splitSnippet", () => {
  it("режет текст по маркерам подсветки", () => {
    expect(splitSnippet("а \u0002ветка\u0003 и \u0002ветки\u0003")).toEqual([
      { text: "а ", hit: false },
      { text: "ветка", hit: true },
      { text: " и ", hit: false },
      { text: "ветки", hit: true },
    ]);
  });
});

describe("linkify", () => {
  it("делает ссылки из [[slug]] и путей rollout, не трогая код", () => {
    const body = "См. [[a]] и [[b]].\n- rollout_summaries/x.md — сводка\n`rollout_summaries/x.md`";
    const out = linkify(body, [
      { text: "a", targetId: "id-a" },
      { text: "b", targetId: "" },
      { text: "rollout_summaries/x.md", targetId: "id-x" },
    ]);
    expect(out).toContain("[a](aim-item:id-a)");
    expect(out).toContain("[b](aim-item:)");
    expect(out).toContain("- [rollout_summaries/x.md](aim-item:id-x) — сводка");
    expect(out).toContain("`rollout_summaries/x.md`");
  });
});
