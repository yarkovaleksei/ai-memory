import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "../App";
import { historyService, plansService, rejectCode } from "../test/fakeBindings";

vi.mock("../api/bindings", async () => (await import("../test/fakeBindings")).fakeBindings());

// CodeMirror в jsdom не измеряет разметку (ADR-013): редактор подменён полем
// ввода с тем же доступным именем, diff — списком содержимого.
vi.mock("./CodeEditor", () => ({
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
vi.mock("./DiffView", () => ({
  DiffView: ({ change }: { change: { before: string; after: string } }) => (
    <pre data-testid="diff">{`${change.before}→${change.after}`}</pre>
  ),
}));

beforeEach(() => {
  window.location.hash = "";
  localStorage.clear();
  vi.clearAllMocks();
});

const openItem = async (title: string) => {
  render(<App />);
  fireEvent.click(await screen.findByText(title));
  // Первый заголовок — название записи; у заметки такой же заголовок есть в тексте.
  const [heading] = await screen.findAllByRole("heading", { name: title });
  return within(heading?.closest(".MuiPaper-root") as HTMLElement);
};

describe("правка факта Claude", () => {
  it("строит план по ID и хэшу черновика, показывает diff и пишет только после подтверждения", async () => {
    const pane = await openItem("Пакеты только через yarn");
    fireEvent.click(pane.getByRole("button", { name: "Изменить" }));

    const dialog = within(await screen.findByRole("dialog", { name: "Факт Claude" }));
    const description = await dialog.findByLabelText("Описание");
    expect(description).toHaveValue("Только yarn");
    fireEvent.change(description, { target: { value: "Только yarn 4" } });
    fireEvent.click(dialog.getByRole("button", { name: "Показать изменения" }));

    const planDialog = within(await screen.findByRole("dialog", { name: "Изменить факт «prefer-yarn»" }));
    expect(plansService.UpdateFact).toHaveBeenCalledWith(
      "i-yarn",
      "h-yarn",
      expect.objectContaining({ name: "prefer-yarn", description: "Только yarn 4" }),
    );
    expect(planDialog.getByText(/prefer-yarn\.md$/)).toBeInTheDocument();
    expect(planDialog.getByTestId("diff")).toHaveTextContent("старое →новое");
    expect(plansService.Apply).not.toHaveBeenCalled();

    fireEvent.click(planDialog.getByRole("button", { name: "Записать" }));
    await waitFor(() => expect(plansService.Apply).toHaveBeenCalledWith("plan-claude.fact.update"));
    expect(await screen.findByText("Записано. Операцию можно откатить в истории")).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("отмена забывает план в Go", async () => {
    const pane = await openItem("Пакеты только через yarn");
    fireEvent.click(pane.getByRole("button", { name: "Удалить" }));
    const planDialog = within(await screen.findByRole("dialog", { name: "Удалить факты Claude: 1" }));
    fireEvent.click(planDialog.getByRole("button", { name: "Отмена" }));
    await waitFor(() => expect(plansService.Discard).toHaveBeenCalledWith("plan-claude.fact.delete"));
    expect(plansService.Apply).not.toHaveBeenCalled();
  });

  it("отказ Go показывается текстом из словаря", async () => {
    plansService.Apply.mockImplementationOnce(() => rejectCode("write.conflict", { path: "/fx/a.md" }));
    const pane = await openItem("Пакеты только через yarn");
    fireEvent.click(pane.getByRole("button", { name: "Удалить" }));
    fireEvent.click(await screen.findByRole("button", { name: "Записать" }));
    expect(
      await screen.findByText(
        "Файл /fx/a.md изменился с момента, как вы его открыли. Откройте запись заново",
      ),
    ).toBeInTheDocument();
  });
});

describe("массовые действия", () => {
  it("удаляет выбранные факты одним планом", async () => {
    render(<App />);
    await screen.findByText("Пакеты только через yarn");
    const row = screen.getByText("Пакеты только через yarn").closest("tr") as HTMLElement;
    fireEvent.click(within(row).getByRole("checkbox"));
    fireEvent.click(await screen.findByRole("button", { name: "Удалить 1 факт" }));
    await screen.findByRole("dialog", { name: "Удалить факты Claude: 1" });
    expect(plansService.DeleteFacts).toHaveBeenCalledWith(["i-yarn"]);
  });
});

describe("записи Codex", () => {
  it("правятся только заметками и показывают статус консолидации", async () => {
    const pane = await openItem("User preferences");
    expect(pane.queryByRole("button", { name: "Изменить" })).not.toBeInTheDocument();
    expect(pane.queryByRole("button", { name: "Удалить" })).not.toBeInTheDocument();
    fireEvent.click(pane.getByRole("button", { name: "Забыть" }));

    const dialog = within(await screen.findByRole("dialog", { name: "Забыть" }));
    expect(dialog.getByLabelText("Заголовок")).toHaveValue("Забыть: User preferences");
    expect(dialog.getByText(/в первом ходе любой своей сессии после/)).toBeInTheDocument();
    fireEvent.click(dialog.getByRole("button", { name: "Показать изменения" }));
    await screen.findByRole("dialog", { name: "Заметка «Забыть»: Забыть: User preferences" });
    expect(plansService.ForgetNote).toHaveBeenCalledWith(
      "Забыть: User preferences",
      "Исключи из памяти «User preferences»: это больше не верно.",
    );
  });

  it("наша заметка правится целиком и помечена «ожидает консолидации»", async () => {
    const pane = await openItem("Сборка демо");
    expect(pane.getByText("Ожидает консолидации")).toBeInTheDocument();
    fireEvent.click(pane.getByRole("button", { name: "Изменить заметку" }));
    const dialog = within(await screen.findByRole("dialog", { name: "Заметка Codex" }));
    fireEvent.change(await dialog.findByLabelText("Заметка целиком"), {
      target: { value: "# Сборка демо\n\n- Ещё.\n" },
    });
    fireEvent.click(dialog.getByRole("button", { name: "Показать изменения" }));
    await waitFor(() =>
      expect(plansService.UpdateSource).toHaveBeenCalledWith("i-note", "h-note", "# Сборка демо\n\n- Ещё.\n"),
    );
  });
});

describe("история", () => {
  it("показывает операции и откатывает через план", async () => {
    window.location.hash = "#/history";
    render(<App />);
    expect(await screen.findByText("Изменить факт «prefer-yarn»")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Откатить" }));
    const planDialog = within(await screen.findByRole("dialog", { name: "Откат: правка факта" }));
    expect(historyService.Revert).toHaveBeenCalledWith(
      "abc1234def",
      expect.stringContaining("Пользователь отменил эту заметку"),
    );
    fireEvent.click(planDialog.getByRole("button", { name: "Записать" }));
    await waitFor(() => expect(plansService.Apply).toHaveBeenCalledWith("plan-revert"));
  });
});

describe("перенос между агентами", () => {
  it("факт Claude переносится в Codex заметкой с указанием источника", async () => {
    const pane = await openItem("Пакеты только через yarn");
    fireEvent.click(pane.getByRole("button", { name: "Перенести…" }));
    const dialog = within(await screen.findByRole("dialog", { name: "Перенос «Пакеты только через yarn»" }));
    // Пара с прошлым переносом в Codex — предупреждение о повторе.
    expect(await dialog.findByText(/уже переносилась сюда: «Сборка демо»/)).toBeInTheDocument();
    const body = dialog.getByLabelText("Текст заметки") as HTMLTextAreaElement;
    expect(body.value).toContain(
      "Источник: «Пакеты только через yarn» из памяти Claude Code, проект /home/me/projects/demo.",
    );
    fireEvent.click(dialog.getByLabelText("Убрать из источника"));
    fireEvent.click(dialog.getByRole("button", { name: "Показать изменения" }));
    await screen.findByRole("dialog", { name: "Перенос в Codex: заметка «Пакеты только через yarn»" });
    expect(plansService.TransferToCodex).toHaveBeenCalledWith(
      "i-yarn",
      "Пакеты только через yarn",
      expect.stringContaining("Ставь пакеты через **yarn**"),
      true,
    );
  });

  it("запись Codex переносится в Claude фактом и заметкой «Забыть»", async () => {
    const pane = await openItem("User preferences");
    fireEvent.click(pane.getByRole("button", { name: "Перенести…" }));
    const dialog = within(await screen.findByRole("dialog", { name: "Перенос «User preferences»" }));
    expect(dialog.queryByRole("button", { name: "В Codex" })).not.toBeInTheDocument();
    expect(dialog.getByLabelText("Имя")).toHaveValue("user-preferences");
    fireEvent.click(dialog.getByLabelText("Убрать из источника"));
    fireEvent.click(dialog.getByRole("button", { name: "Показать изменения" }));
    await waitFor(() =>
      expect(plansService.TransferToClaude).toHaveBeenCalledWith(
        "i-section",
        "claude:/fx/.claude",
        "",
        expect.objectContaining({ name: "user-preferences", body: "- Prefers short answers." }),
        true,
        "Забыть: User preferences",
        expect.stringContaining("перенёс «User preferences» в память Claude Code"),
      ),
    );
  });

  it("показывает пару «источник — копия» и открывает копию", async () => {
    const pane = await openItem("Пакеты только через yarn");
    fireEvent.click(await pane.findByRole("button", { name: "Скопировано в Codex: «Сборка демо»" }));
    expect((await screen.findAllByRole("heading", { name: "Сборка демо" }))[0]).toBeInTheDocument();
  });
});
