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

describe("перенос правила между инструкциями", () => {
  it("раздел AGENTS.md вставляется в CLAUDE.md через план с diff", async () => {
    plansService.CopySection.mockImplementationOnce(() =>
      Promise.resolve({
        ...plan("instructions.section-move", {
          heading: "Код",
          from: "AGENTS.md",
          to: "CLAUDE.md",
          agent: "claude",
        }),
        warnings: [{ code: "plan.section-imported", params: { file: "CLAUDE.md", import: "AGENTS.md" } }],
      }),
    );
    render(<App />);
    const pane = await openPreview("AGENTS.md");
    fireEvent.click(pane.getByRole("button", { name: "Перенести правило" }));
    const dialog = within(await screen.findByRole("dialog", { name: "Перенести правило из AGENTS.md" }));
    // По умолчанию — первый подраздел: верхний заголовок накрывает весь файл.
    expect(await dialog.findByRole("combobox", { name: "Раздел" })).toHaveTextContent("Код");
    expect(dialog.getByLabelText("Текст раздела")).toHaveTextContent("- yarn");
    expect(dialog.getByRole("combobox", { name: "Куда" })).toHaveTextContent("Claude Code · CLAUDE.md");

    fireEvent.mouseDown(dialog.getByRole("combobox", { name: "Место" }));
    fireEvent.click(
      await screen.findByRole("option", { name: /В конец раздела «.*Только для Claude Code»/ }),
    );
    fireEvent.click(dialog.getByLabelText("Убрать из исходного файла"));
    fireEvent.click(dialog.getByRole("button", { name: "Показать изменения" }));

    const planDialog = within(
      await screen.findByRole("dialog", { name: "Перенос правила «Код» из AGENTS.md в CLAUDE.md" }),
    );
    expect(plansService.CopySection).toHaveBeenLastCalledWith(
      "i-agents",
      "h-agents",
      1,
      "i-claude-md",
      "h-claude",
      1,
      true,
    );
    expect(planDialog.getByText(/Claude Code увидит правило дважды/)).toBeInTheDocument();
    fireEvent.click(planDialog.getByRole("button", { name: "Записать" }));
    await waitFor(() => expect(plansService.Apply).toHaveBeenCalled());
  });

  it("импорты @ в Codex предупреждают, отсутствующий AGENTS.md будет создан", async () => {
    plansService.InstructionSections.mockImplementationOnce(() =>
      Promise.resolve({
        hash: "h",
        sections: [
          {
            index: 0,
            level: 2,
            heading: "Общее",
            text: "## Общее\n\n@docs/style.md",
            imports: ["docs/style.md"],
          },
        ],
      }),
    );
    plansService.SectionTargets.mockImplementationOnce(() =>
      Promise.resolve([
        {
          key: "new:codex-projects",
          agent: "codex",
          title: "AGENTS.md",
          file: "/home/me/projects/demo/AGENTS.md",
          role: "main",
          exists: false,
          repo: "",
          hash: "",
          sections: [],
        },
      ]),
    );
    render(<App />);
    const pane = await openPreview("AGENTS.md");
    fireEvent.click(pane.getByRole("button", { name: "Перенести правило" }));
    const dialog = within(await screen.findByRole("dialog", { name: "Перенести правило из AGENTS.md" }));
    expect(await dialog.findByRole("combobox", { name: "Куда" })).toHaveTextContent(
      "Codex · AGENTS.md — будет создан",
    );
    expect(dialog.getByText(/импорты docs\/style\.md: Codex их не понимает/)).toBeInTheDocument();
    // В новый файл вставлять можно только в конец.
    expect(dialog.getByRole("combobox", { name: "Место" })).toHaveAttribute("aria-disabled", "true");
  });
});

describe("AGENTS.md основным", () => {
  it("предлагает приём и переносит общие блоки CLAUDE.md в AGENTS.md", async () => {
    const state = {
      state: "both",
      claudeFile: "/home/me/projects/demo/CLAUDE.md",
      agentsFile: "/home/me/projects/demo/AGENTS.md",
      claudeExists: true,
      agentsExists: true,
      claudeHash: "h-claude",
      agentsHash: "h-agents",
      import: "@AGENTS.md",
      override: false,
      blocks: [
        {
          index: 0,
          level: 1,
          heading: "Проект demo",
          text: "# Проект demo",
          imports: [] as string[],
          intro: true,
        },
        {
          index: 1,
          level: 2,
          heading: "Код",
          text: "## Код\n\n- yarn",
          imports: [] as string[],
          intro: false,
        },
        {
          index: 2,
          level: 2,
          heading: "Стиль",
          text: "## Стиль\n\n@docs/style.md",
          imports: ["docs/style.md"],
          intro: false,
        },
        {
          index: 3,
          level: 2,
          heading: "Только для Claude Code",
          text: "## Только для Claude Code",
          imports: [] as string[],
          intro: false,
        },
      ],
    };
    plansService.UnifyDraft.mockImplementation(() => Promise.resolve(state));
    try {
      render(<App />);
      const pane = await openPreview("AGENTS.md");
      expect(await pane.findByText(/CLAUDE.md и AGENTS.md проекта живут порознь/)).toBeInTheDocument();
      fireEvent.click(pane.getByRole("button", { name: "Сделать AGENTS.md основным" }));
      const dialog = within(
        await screen.findByRole("dialog", { name: "AGENTS.md — основной, CLAUDE.md импортирует его" }),
      );
      // Общее — отмечено; заголовок документа, раздел с импортом и раздел для Claude — нет.
      expect(await dialog.findByLabelText("Код")).toBeChecked();
      for (const name of ["Проект demo", "Стиль", "Только для Claude Code"]) {
        expect(dialog.getByLabelText(name)).not.toBeChecked();
      }
      fireEvent.click(dialog.getByLabelText("Стиль"));
      expect(dialog.getByText(/импорты docs\/style\.md/)).toBeInTheDocument();
      fireEvent.click(dialog.getByRole("button", { name: "Показать изменения" }));
      expect(
        await screen.findByRole("dialog", { name: "AGENTS.md — основной в проекте demo" }),
      ).toBeInTheDocument();
      expect(plansService.Unify).toHaveBeenLastCalledWith(
        "path:/home/me/projects/demo",
        "h-claude",
        "h-agents",
        [1, 2],
      );
    } finally {
      plansService.UnifyDraft.mockReset();
      plansService.UnifyDraft.mockImplementation(() =>
        Promise.resolve({ ...state, state: "done", blocks: [] }),
      );
    }
  });
});
