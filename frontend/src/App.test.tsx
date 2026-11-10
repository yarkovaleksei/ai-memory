import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import { listItems } from "./test/fakeBindings";

vi.mock("./api/bindings", async () => (await import("./test/fakeBindings")).fakeBindings());

beforeEach(() => {
  window.location.hash = "";
  localStorage.clear();
  listItems.mockClear();
});

describe("главный экран", () => {
  it("показывает дерево проектов, скрывает временные и открывает запись", async () => {
    render(<App />);

    expect(await screen.findByText("Пакеты только через yarn")).toBeInTheDocument();
    expect(screen.getByText("Релизный процесс")).toBeInTheDocument();
    // Временный проект свёрнут и его записи не в общем списке.
    expect(screen.queryByText("Черновик эксперимента")).not.toBeInTheDocument();
    expect(screen.getByText("Временные")).toBeInTheDocument();
    expect(screen.queryByText("scratch-42")).not.toBeInTheDocument();
    expect(screen.getByText("Глобальная память")).toBeInTheDocument();

    fireEvent.click(screen.getByText("Пакеты только через yarn"));
    const preview = await screen.findByRole("heading", { name: "Пакеты только через yarn" });
    const pane = within(preview.closest(".MuiPaper-root") as HTMLElement);
    expect(pane.getByText("yarn", { selector: "strong" })).toBeInTheDocument();
    expect(pane.getByText("Указания")).toBeInTheDocument();

    // Ссылка [[release-flow]] открывает другую запись, неразрешённая — нет.
    expect(pane.getByText("missing-slug").closest("button")).toBeNull();
    fireEvent.click(pane.getByRole("button", { name: "release-flow" }));
    expect(await screen.findByRole("heading", { name: "Релизный процесс" })).toBeInTheDocument();
    expect(screen.getByText("Файла нет в индексе MEMORY.md")).toBeInTheDocument();
  });

  it("выбор проекта и временной группы фильтрует список", async () => {
    render(<App />);
    await screen.findByText("Пакеты только через yarn");

    fireEvent.click(screen.getByText("Временные"));
    fireEvent.click(await screen.findByText("scratch-42"));
    expect(await screen.findByText("Черновик эксперимента")).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText("Пакеты только через yarn")).not.toBeInTheDocument());
    expect(listItems).toHaveBeenLastCalledWith(
      expect.objectContaining({ projectId: "path:/tmp/scratch-42" }),
    );
  });

  it("поиск подсвечивает совпадение в сниппете", async () => {
    render(<App />);
    await screen.findByText("Пакеты только через yarn");

    fireEvent.change(screen.getByPlaceholderText("Поиск"), { target: { value: "yarn" } });
    const mark = await screen.findByText("yarn", { selector: "mark" });
    expect(mark).toBeInTheDocument();
    expect(listItems).toHaveBeenLastCalledWith(expect.objectContaining({ q: "yarn" }));
    expect(screen.queryByText("Релизный процесс")).not.toBeInTheDocument();
  });
});

describe("О программе", () => {
  it("показывает сведения от Go-сервиса и хранилища", async () => {
    window.location.hash = "#/about";
    render(<App />);

    expect(await screen.findByText("Версия 9.9.9-test")).toBeInTheDocument();
    expect(screen.getByText("Профили Claude Code")).toBeInTheDocument();
    expect(screen.getByText(/\/fx\/home\/\.claude-work/)).toBeInTheDocument();
    expect(screen.getByText("/fx/home/.codex")).toBeInTheDocument();
    const sources = (await screen.findByText("Найденные хранилища")).parentElement as HTMLElement;
    expect(within(sources).getByText(".claude")).toBeInTheDocument();
  });
});
