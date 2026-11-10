import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "../App";
import { listItems } from "../test/fakeBindings";

vi.mock("../api/bindings", async () => (await import("../test/fakeBindings")).fakeBindings());

beforeEach(() => {
  window.location.hash = "";
  localStorage.clear();
  listItems.mockClear();
});

const openPreview = async (title: string) => {
  fireEvent.click(await screen.findByText(title));
  const heading = await screen.findByRole("heading", { name: title });
  return within(heading.closest(".MuiPaper-root") as HTMLElement);
};

describe("инструкции и навыки", () => {
  it("стоят в списке рядом с памятью, раздел их отделяет", async () => {
    render(<App />);
    const row = (await screen.findByText("AGENTS.md")).closest("tr") as HTMLElement;
    expect(within(row).getByText("Инструкции")).toBeInTheDocument();
    expect(screen.getByText("Пакеты только через yarn")).toBeInTheDocument();
    // Инструкции не отмечаются: объединять и удалять их как память нельзя.
    expect(within(row).queryByRole("checkbox")).not.toBeInTheDocument();

    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Раздел" }));
    fireEvent.click(await screen.findByRole("option", { name: "Навыки" }));
    await waitFor(() => expect(screen.queryByText("AGENTS.md")).not.toBeInTheDocument());
    expect(screen.getByText("commit-message")).toBeInTheDocument();
    expect(screen.queryByText("Пакеты только через yarn")).not.toBeInTheDocument();
    expect(listItems).toHaveBeenLastCalledWith(expect.objectContaining({ kind: "skill" }));
  });

  it("у AGENTS.md нет действий памяти, видно, что файл в git-репозитории", async () => {
    render(<App />);
    const pane = await openPreview("AGENTS.md");
    expect(pane.getByText("Основной файл")).toBeInTheDocument();
    expect(pane.getByText("В git-репозитории проекта")).toBeInTheDocument();
    expect(pane.getByText("Инструкции и навыки Codex в проектах")).toBeInTheDocument();
    for (const name of ["Забыть", "Уточнить заметкой", "Перенести"]) {
      expect(pane.queryByRole("button", { name })).not.toBeInTheDocument();
    }
  });

  it("общий навык показывает, где ещё подключён его каталог", async () => {
    render(<App />);
    fireEvent.mouseDown(await screen.findByRole("combobox", { name: "Раздел" }));
    fireEvent.click(await screen.findByRole("option", { name: "Навыки" }));
    const pane = await openPreview("commit-message");
    expect(pane.getByText("Общий навык")).toBeInTheDocument();
    expect(pane.getByText("Навык пользователя")).toBeInTheDocument();
    expect(pane.getByText("/fx/.agents/skills/commit-message", { selector: "p" })).toBeInTheDocument();
    fireEvent.click(pane.getByRole("button", { name: "/fx/.agents/skills/commit-message" }));
    expect(await screen.findByRole("heading", { name: "commit-message (Codex)" })).toBeInTheDocument();
  });
});
