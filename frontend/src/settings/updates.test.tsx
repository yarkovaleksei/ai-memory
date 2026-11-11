import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "../App";
import { Browser, Events } from "../api/bindings";
import { updatesService, updateView } from "../test/fakeBindings";

vi.mock("../api/bindings", async () => (await import("../test/fakeBindings")).fakeBindings());

beforeEach(() => {
  window.location.hash = "#/settings";
  localStorage.clear();
  vi.clearAllMocks();
});

// Статус, который Go присылает событием update:status.
const emit = (data: ReturnType<typeof updateView>) => {
  const calls = vi.mocked(Events.On).mock.calls as unknown as [string, (ev: { data: unknown }) => void][];
  const handler = calls.filter(([name]) => name === "update:status").at(-1)?.[1];
  expect(handler).toBeDefined();
  act(() => handler!({ data }));
};

describe("обновление ai-memory в настройках", () => {
  it("проверяет, скачивает и открывает папку с пакетом по кнопкам", async () => {
    render(<App />);
    expect(await screen.findByText("Установлена версия 0.6.0")).toBeInTheDocument();
    // Без кнопки в сеть не ходим.
    expect(updatesService.Check).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Проверить обновления" }));
    expect(await screen.findByText(/Доступна версия 0\.7\.0, пакет 12,3/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Что нового" }));
    expect(Browser.OpenURL).toHaveBeenCalledWith(
      "https://github.com/yarkovaleksei/ai-memory/releases/tag/v0.7.0",
    );

    fireEvent.click(screen.getByRole("button", { name: "Скачать" }));
    await waitFor(() => expect(updatesService.Download).toHaveBeenCalled());
    emit(updateView({ state: "downloading", latest: "0.7.0", size: 12_300_000, received: 6_150_000 }));
    // Кэш запроса оповещает компонент не сразу: ждём через findBy.
    expect(await screen.findByRole("progressbar", { name: "Скачивание пакета" })).toHaveAttribute(
      "aria-valuenow",
      "50",
    );
    expect(screen.getByRole("button", { name: "Отменить" })).toBeInTheDocument();

    const command = "sudo apt install /fx/data/updates/0.7.0/ai-memory_0.7.0_amd64.deb";
    emit(
      updateView({
        state: "ready",
        latest: "0.7.0",
        package: "ai-memory_0.7.0_amd64.deb",
        install: "command",
        command,
      }),
    );
    expect(await screen.findByText(command)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Открыть папку с пакетом" }));
    await waitFor(() => expect(updatesService.Install).toHaveBeenCalled());
  });

  it("сборка не из пакета обновляется вручную и не предлагает проверку", async () => {
    updatesService.Status.mockResolvedValueOnce(
      updateView({ state: "unsupported", current: "0.0.1-dev", format: "" }),
    );
    render(<App />);
    expect(await screen.findByText(/обновляется вручную/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Проверить обновления" })).not.toBeInTheDocument();
  });

  it("показывает ошибку сверки текстом из словаря и даёт скачать снова", async () => {
    updatesService.Status.mockResolvedValueOnce(
      updateView({
        state: "failed",
        latest: "0.7.0",
        package: "ai-memory_0.7.0_amd64.deb",
        error: { code: "update.mismatch", params: { package: "ai-memory_0.7.0_amd64.deb" } },
      }),
    );
    render(<App />);
    expect(
      await screen.findByText(/Пакет ai-memory_0\.7\.0_amd64\.deb не совпал с контрольной суммой/),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Скачать снова" }));
    await waitFor(() => expect(updatesService.Download).toHaveBeenCalled());
  });
});
