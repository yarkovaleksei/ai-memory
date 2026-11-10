import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "../App";
import { settingsService } from "../test/fakeBindings";

vi.mock("../api/bindings", async () => (await import("../test/fakeBindings")).fakeBindings());

beforeEach(() => {
  window.location.hash = "#/settings";
  localStorage.clear();
  vi.clearAllMocks();
});

describe("экран «Настройки»", () => {
  it("добавляет профиль Claude через выбор каталога, убирает другой и сохраняет", async () => {
    render(<App />);
    expect(await screen.findByText("~/.claude-personal")).toBeInTheDocument();
    expect(screen.getByText("/fx/home/.claude")).toBeInTheDocument();
    expect(screen.getByText(/сейчас v2: из/)).toBeInTheDocument();

    // Выбор каталога вписывает путь в поле, «Добавить» добавляет профиль.
    fireEvent.click(screen.getAllByRole("button", { name: "Выбрать каталог" })[0]!);
    await waitFor(() =>
      expect(screen.getByRole("textbox", { name: "Путь к профилю (можно с ~)" })).toHaveValue(
        "/fx/home/.claude-work",
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: "Добавить" }));
    expect(await screen.findByText("/fx/home/.claude-work")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Убрать ~/.claude-personal" }));
    fireEvent.click(screen.getByRole("button", { name: "Сохранить" }));

    await waitFor(() =>
      expect(settingsService.Save).toHaveBeenCalledWith(
        expect.objectContaining({ extraClaudeProfiles: ["/fx/home/.claude-work"] }),
      ),
    );
    expect(await screen.findByText("Настройки сохранены, память перечитана")).toBeInTheDocument();
  });
});
