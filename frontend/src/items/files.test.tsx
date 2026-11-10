import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "../App";
import { listItems, plan, plansService } from "../test/fakeBindings";

vi.mock("../api/bindings", async () => (await import("../test/fakeBindings")).fakeBindings());

// CodeMirror в jsdom не измеряет разметку (ADR-013): редактор и diff подменены.
vi.mock("../edit/CodeEditor", () => ({
  CodeEditor: ({
    value,
    onChange,
    label,
  }: {
    value: string;
    onChange: (v: string) => void;
    label: string;
  }) => <textarea aria-label={label} value={value} onChange={(e) => onChange(e.target.value)} />,
}));
vi.mock("../edit/DiffView", () => ({
  DiffView: ({ change }: { change: { before: string; after: string } }) => (
    <pre data-testid="diff">{`${change.before}→${change.after}`}</pre>
  ),
}));

beforeEach(() => {
  window.location.hash = "";
  localStorage.clear();
  listItems.mockClear();
  plansService.Apply.mockClear();
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

describe("правка инструкций и навыков", () => {
  it("AGENTS.md правится исходником, план предупреждает о git-репозитории", async () => {
    plansService.Draft.mockImplementationOnce(() =>
      Promise.resolve({ content: "# Правила\n", hash: "h-agents", fact: null }),
    );
    plansService.UpdateSource.mockImplementationOnce(() =>
      Promise.resolve({
        ...plan("instructions.update", { file: "AGENTS.md", agent: "codex" }),
        warnings: [{ code: "plan.in-repo", params: { repo: "/home/me/projects/demo" } }],
      }),
    );
    render(<App />);
    const pane = await openPreview("AGENTS.md");
    fireEvent.click(pane.getByRole("button", { name: "Изменить" }));
    const editor = within(await screen.findByRole("dialog", { name: "Инструкции AGENTS.md" }));
    const field = await editor.findByLabelText("Исходник файла");
    fireEvent.change(field, { target: { value: "# Правила\n\n- Новое.\n" } });
    fireEvent.click(editor.getByRole("button", { name: "Показать изменения" }));

    const dialog = within(await screen.findByRole("dialog", { name: "Изменить инструкции AGENTS.md" }));
    expect(plansService.UpdateSource).toHaveBeenLastCalledWith(
      "i-agents",
      "h-agents",
      "# Правила\n\n- Новое.\n",
    );
    expect(dialog.getByText(/лежит в git-репозитории \/home\/me\/projects\/demo/)).toBeInTheDocument();
    fireEvent.click(dialog.getByRole("button", { name: "Записать" }));
    await waitFor(() => expect(plansService.Apply).toHaveBeenCalled());
  });

  it("общий навык записывается только после отдельного подтверждения", async () => {
    plansService.Draft.mockImplementationOnce(() =>
      Promise.resolve({ content: "---\nname: commit-message\n---\n", hash: "h-skill", fact: null }),
    );
    plansService.UpdateSource.mockImplementationOnce(() =>
      Promise.resolve({
        ...plan("skill.update", { name: "commit-message", agent: "claude" }),
        warnings: [
          { code: "plan.shared-skill", params: { dir: "/fx/.agents/skills/commit-message" }, confirm: true },
        ],
      }),
    );
    render(<App />);
    fireEvent.mouseDown(await screen.findByRole("combobox", { name: "Раздел" }));
    fireEvent.click(await screen.findByRole("option", { name: "Навыки" }));
    const pane = await openPreview("commit-message");
    fireEvent.click(pane.getByRole("button", { name: "Изменить" }));
    const editor = within(await screen.findByRole("dialog", { name: "Навык «commit-message»" }));
    fireEvent.change(await editor.findByLabelText("Исходник файла"), { target: { value: "x" } });
    fireEvent.click(editor.getByRole("button", { name: "Показать изменения" }));

    const dialog = within(await screen.findByRole("dialog", { name: "Изменить навык «commit-message»" }));
    const apply = dialog.getByRole("button", { name: "Записать" });
    expect(apply).toBeDisabled();
    fireEvent.click(dialog.getByLabelText("Понимаю, записать всё равно"));
    expect(apply).toBeEnabled();
  });
});
