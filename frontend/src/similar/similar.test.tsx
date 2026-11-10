import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "../App";
import { llmService, plansService } from "../test/fakeBindings";

vi.mock("../api/bindings", async () => (await import("../test/fakeBindings")).fakeBindings());
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
vi.mock("../edit/DiffView", () => ({ DiffView: () => <pre /> }));

beforeEach(() => {
  window.location.hash = "#/similar";
  localStorage.clear();
  vi.clearAllMocks();
});

// openMerge открывает «Объединить…», когда статус записи уже получен: до
// этого кнопка выключена.
const openMerge = async () => {
  await screen.findByText("Сходство 47%");
  const button = screen.getByRole("button", { name: "Объединить…" });
  await waitFor(() => expect(button).toBeEnabled());
  fireEvent.click(button);
  return within(await screen.findByRole("dialog", { name: "Объединить 2 записи" }));
};

describe("экран «Похожие»", () => {
  it("показывает группу, сравнивает и объединяет одним планом", async () => {
    render(<App />);
    expect(await screen.findByText("Сходство 47%")).toBeInTheDocument();
    expect(screen.getByText("Пакеты только через yarn")).toBeInTheDocument();
    expect(screen.getByText("Сборка демо")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Сравнить" }));
    expect(await screen.findByText("make demo.", { exact: false })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Объединить…" }));
    const dialog = within(await screen.findByRole("dialog", { name: "Объединить 2 записи" }));
    expect(dialog.getByLabelText("Имя")).toHaveValue("prefer-yarn");
    expect(
      dialog.getByText(/удаляются факты Claude: 0; для записей Codex создаются заметки «Забыть»: 1/),
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
    expect(
      await screen.findByRole("dialog", { name: "Объединение записей (2) в «claude»" }),
    ).toBeInTheDocument();
  });

  it("объединение недоступно, если снята отметка со всех, кроме одной", async () => {
    render(<App />);
    await screen.findByText("Сходство 47%");
    fireEvent.click(screen.getByRole("checkbox", { name: "Сборка демо" }));
    expect(screen.getByRole("button", { name: "Объединить…" })).toBeDisabled();
  });

  it("вписывает черновик от LLM в форму и показывает расхождения", async () => {
    render(<App />);
    const dialog = await openMerge();
    fireEvent.click(await dialog.findByRole("button", { name: "Черновик от Claude" }));
    await waitFor(() =>
      expect(dialog.getByLabelText("Текст факта")).toHaveValue("Ставить пакеты только через yarn 4.\n"),
    );
    expect(llmService.MergeDraft).toHaveBeenCalledWith(["i-yarn", "i-note"], "claude");
    expect(dialog.getByLabelText("Описание")).toHaveValue("Чем ставить пакеты в демо");
    expect(dialog.getByLabelText("Имя")).toHaveValue("prefer-yarn");
    expect(
      dialog.getByText(/Разные версии yarn \(«Пакеты только через yarn», «Сборка демо»\)/),
    ).toBeInTheDocument();

    // Черновик переживает смену цели: заметка Codex получает тот же текст.
    fireEvent.click(dialog.getByRole("button", { name: "Итог — заметка Codex" }));
    expect(dialog.getByLabelText("Заголовок")).toHaveValue("Менеджер пакетов");
    expect(dialog.getByLabelText("Текст заметки")).toHaveValue("Ставить пакеты только через yarn 4.\n");
  });

  it("показывает ошибку CLI в диалоге объединения", async () => {
    llmService.MergeDraft.mockImplementationOnce(() =>
      Promise.reject(
        Object.assign(new Error("x"), { cause: { code: "llm.not-found", params: { agent: "codex" } } }),
      ),
    );
    render(<App />);
    const dialog = await openMerge();
    fireEvent.click(await dialog.findByRole("button", { name: "Черновик от Codex" }));
    expect(await dialog.findByText(/Не найдена команда codex в PATH/)).toBeInTheDocument();
  });

  it("без CLI агентов кнопок модели нет", async () => {
    llmService.Agents.mockImplementationOnce(() => Promise.resolve([]));
    render(<App />);
    expect(await screen.findByText(/черновик от модели недоступен/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Проверить через Claude" })).not.toBeInTheDocument();
  });

  it("строит отчёт о противоречиях по группам экрана", async () => {
    render(<App />);
    await screen.findByText("Сходство 47%");
    fireEvent.click(screen.getByRole("switch", { name: "Только между агентами" }));
    fireEvent.click(await screen.findByRole("button", { name: "Проверить через Codex" }));
    expect(await screen.findByText("Разные базовые ветки релиза")).toBeInTheDocument();
    expect(llmService.Contradictions).toHaveBeenCalledWith(
      expect.objectContaining({ crossAgent: true, minScore: 0.2 }),
      "codex",
    );
    expect(screen.getByText("Проверено групп: 1 из 2")).toBeInTheDocument();
    expect(screen.getByText(/Не все группы поместились/)).toBeInTheDocument();
  });
});
