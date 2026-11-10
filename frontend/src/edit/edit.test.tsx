import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "../App";
import {
  historyService,
  items,
  listSources,
  llmService,
  plansService,
  rejectCode,
  sources,
} from "../test/fakeBindings";

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
    expect(await screen.findByText("Записано. Операцию можно откатить в истории.")).toBeInTheDocument();
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
        "Файл /fx/a.md изменился с момента, как вы его открыли. Откройте запись заново.",
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
    const del = await screen.findByRole("button", { name: "Удалить 1 факт" });
    await waitFor(() => expect(del).toBeEnabled());
    fireEvent.click(del);
    await screen.findByRole("dialog", { name: "Удалить факты Claude: 1" });
    expect(plansService.DeleteFacts).toHaveBeenCalledWith(["i-yarn"]);
  });

  it("объединяет отмеченные записи с черновиком от модели", async () => {
    render(<App />);
    await selectRows("Пакеты только через yarn", "Сборка демо");
    // Удалить можно только факты Claude, а отмечена и заметка Codex.
    expect(screen.getByRole("button", { name: "Удалить 2 факта" })).toBeDisabled();
    const merge = screen.getByRole("button", { name: "Объединить" });
    await waitFor(() => expect(merge).toBeEnabled());
    fireEvent.click(merge);

    const dialog = within(await screen.findByRole("dialog", { name: "Объединить 2 записи" }));
    expect(dialog.getByLabelText("Имя")).toHaveValue("prefer-yarn");
    fireEvent.click(await dialog.findByRole("button", { name: "Черновик от Claude" }));
    await waitFor(() =>
      expect(dialog.getByLabelText("Текст факта")).toHaveValue("Ставить пакеты только через yarn 4.\n"),
    );
    expect(llmService.MergeDraft).toHaveBeenCalledWith(["i-yarn", "i-note"], "claude");
    expect(
      dialog.getByText(/Разные версии yarn \(«Пакеты только через yarn», «Сборка демо»\)/),
    ).toBeInTheDocument();

    fireEvent.click(dialog.getByRole("button", { name: "Показать изменения" }));
    await waitFor(() =>
      expect(plansService.Merge).toHaveBeenCalledWith(
        expect.objectContaining({
          itemIds: ["i-yarn", "i-note"],
          target: "claude",
          forget: [expect.objectContaining({ itemId: "i-note", title: "Забыть: Сборка демо" })],
        }),
      ),
    );
  });

  it("ошибка CLI показывается в диалоге объединения", async () => {
    llmService.MergeDraft.mockImplementationOnce(() =>
      Promise.reject(
        Object.assign(new Error("x"), { cause: { code: "llm.not-found", params: { agent: "codex" } } }),
      ),
    );
    render(<App />);
    await selectRows("Пакеты только через yarn", "Сборка демо");
    const merge = screen.getByRole("button", { name: "Объединить" });
    await waitFor(() => expect(merge).toBeEnabled());
    fireEvent.click(merge);
    const dialog = within(await screen.findByRole("dialog", { name: "Объединить 2 записи" }));
    fireEvent.click(await dialog.findByRole("button", { name: "Черновик от Codex" }));
    expect(await dialog.findByText(/Не найдена команда codex в PATH/)).toBeInTheDocument();
  });

  it("ищет противоречия только среди отмеченных записей", async () => {
    render(<App />);
    await selectRows("Пакеты только через yarn", "Сборка демо");
    const check = screen.getByRole("button", { name: "Найти противоречия" });
    await waitFor(() => expect(check).toBeEnabled());
    fireEvent.click(check);
    const dialog = within(await screen.findByRole("dialog", { name: "Противоречия среди 2 записей" }));
    fireEvent.click(await dialog.findByRole("button", { name: "Проверить через Codex" }));
    expect(await dialog.findByText("Разные базовые ветки релиза")).toBeInTheDocument();
    expect(llmService.Contradictions).toHaveBeenCalledWith(["i-yarn", "i-note"], "codex");
    expect(dialog.getByRole("button", { name: "Объединить" })).toBeInTheDocument();
  });

  it("инструкции отмечаются для проверки дублей, но не для объединения", async () => {
    llmService.Contradictions.mockImplementationOnce((ids: string[]) =>
      Promise.resolve([
        {
          summary: "Правило про yarn уже есть в AGENTS.md",
          kind: "duplicate",
          items: items.filter((it) => ids.includes(it.id)),
        },
      ]),
    );
    render(<App />);
    await selectRows("Пакеты только через yarn", "AGENTS.md");
    const check = screen.getByRole("button", { name: "Найти противоречия" });
    await waitFor(() => expect(check).toBeEnabled());
    expect(screen.getByRole("button", { name: "Объединить" })).toBeDisabled();
    fireEvent.click(check);
    const dialog = within(await screen.findByRole("dialog", { name: "Противоречия среди 2 записей" }));
    fireEvent.click(await dialog.findByRole("button", { name: "Проверить через Codex" }));
    expect(await dialog.findByText("Дубль")).toBeInTheDocument();
    expect(llmService.Contradictions).toHaveBeenLastCalledWith(["i-yarn", "i-agents"], "codex");
    expect(dialog.getByRole("button", { name: "Объединить" })).toBeDisabled();
    fireEvent.click(dialog.getByRole("button", { name: "Удалить факт «Пакеты только через yarn»" }));
    await waitFor(() => expect(plansService.DeleteFacts).toHaveBeenLastCalledWith(["i-yarn"]));
  });

  it("без CLI агентов кнопок модели нет", async () => {
    llmService.Agents.mockImplementationOnce(() => Promise.resolve([]));
    render(<App />);
    await selectRows("Пакеты только через yarn", "Сборка демо");
    const check = screen.getByRole("button", { name: "Найти противоречия" });
    await waitFor(() => expect(check).toBeEnabled());
    fireEvent.click(check);
    const dialog = within(await screen.findByRole("dialog", { name: "Противоречия среди 2 записей" }));
    expect(await dialog.findByText(/запросы к модели недоступны/)).toBeInTheDocument();
    expect(dialog.queryByRole("button", { name: "Проверить через Claude" })).not.toBeInTheDocument();
  });
});

// selectRows отмечает строки списка записей по заголовкам.
const selectRows = async (...titles: string[]) => {
  for (const title of titles) {
    const row = (await screen.findByText(title)).closest("tr") as HTMLElement;
    fireEvent.click(within(row).getByRole("checkbox"));
  }
};

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
    fireEvent.click(pane.getByRole("button", { name: "Перенести" }));
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
    fireEvent.click(pane.getByRole("button", { name: "Перенести" }));
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

describe("нет одного из агентов", () => {
  it("без Codex нет действий, которые пишут в Codex", async () => {
    listSources.mockImplementation(() => Promise.resolve(sources.filter((src) => src.agent === "claude")));
    render(<App />);
    fireEvent.click(await screen.findByText("Пакеты только через yarn"));
    const preview = await screen.findByRole("heading", { name: "Пакеты только через yarn" });
    const pane = within(preview.closest(".MuiPaper-root") as HTMLElement);
    // Факт Claude можно перенести в другой проект Claude, но не в Codex.
    fireEvent.click(pane.getByRole("button", { name: "Перенести" }));
    const dialog = within(await screen.findByRole("dialog"));
    expect(dialog.getByRole("button", { name: "В Claude Code" })).toBeInTheDocument();
    expect(dialog.queryByRole("button", { name: "В Codex" })).not.toBeInTheDocument();
    fireEvent.click(dialog.getByRole("button", { name: "Отмена" }));

    fireEvent.click(screen.getByRole("button", { name: "Создать" }));
    expect(await screen.findByRole("menuitem", { name: "Факт Claude" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Заметка Codex" })).not.toBeInTheDocument();
    listSources.mockImplementation(() => Promise.resolve(sources));
  });

  it("без Claude запись Codex не переносится, создаётся только заметка", async () => {
    listSources.mockImplementation(() => Promise.resolve(sources.filter((src) => src.agent === "codex")));
    render(<App />);
    fireEvent.click(await screen.findByText("Сборка демо"));
    // Заголовок есть и в тексте заметки: первый — заголовок просмотра.
    const preview = (await screen.findAllByRole("heading", { name: "Сборка демо" }))[0]!;
    const pane = within(preview.closest(".MuiPaper-root") as HTMLElement);
    await pane.findByRole("button", { name: "Забыть" });
    expect(pane.queryByRole("button", { name: "Перенести" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Создать" }));
    expect(await screen.findByRole("menuitem", { name: "Заметка Codex" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Факт Claude" })).not.toBeInTheDocument();
    listSources.mockImplementation(() => Promise.resolve(sources));
  });
});
