import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { App } from "./App";

// Подменяем привязки Wails: в тестах нет Go-рантайма.
vi.mock("./api/bindings", () => ({
  InfoService: {
    Info: () =>
      Promise.resolve({
        version: "9.9.9-test",
        configDir: "/fx/cfg",
        dataDir: "/fx/data",
        claudeProfiles: ["/fx/home/.claude", "/fx/home/.claude-work"],
        codexHome: "/fx/home/.codex",
      }),
  },
}));

describe("Dashboard", () => {
  it("показывает сведения от Go-сервиса на русском", async () => {
    render(<App />);

    expect(await screen.findByText("Версия 9.9.9-test")).toBeInTheDocument();
    expect(screen.getByText("Профили Claude Code")).toBeInTheDocument();
    expect(screen.getByText(/\/fx\/home\/\.claude-work/)).toBeInTheDocument();
    expect(screen.getByText("/fx/home/.codex")).toBeInTheDocument();
    expect(screen.getByText("Каталог данных ai-memory")).toBeInTheDocument();
  });
});
