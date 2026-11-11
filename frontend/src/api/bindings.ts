// Единственная точка импорта сгенерированных привязок Wails и рантайма: длинный путь
// пакета Go живёт только здесь, а тесты подменяют этот модуль целиком.
export {
  HistoryService,
  InfoService,
  LibraryService,
  LLMService,
  PlansService,
  SettingsService,
  UpdatesService,
} from "../../bindings/github.com/yarkovaleksei/ai-memory/internal/app";
export type {
  AppInfo,
  ApplyResult,
  CodexStatus,
  Contradiction,
  Draft,
  EditStatus,
  ItemPage,
  ItemQuery,
  ListItem,
  MergeRequest,
  NoteText,
  SectionTarget,
  SectionsDraft,
  SettingsView,
  SkillTarget,
  TransferLink,
} from "../../bindings/github.com/yarkovaleksei/ai-memory/internal/app";
export { Agent } from "../../bindings/github.com/yarkovaleksei/ai-memory/internal/model";
export type {
  Item,
  Link,
  Project,
  Source,
  Warning,
} from "../../bindings/github.com/yarkovaleksei/ai-memory/internal/model";
export type {
  Change,
  Entry,
  Op,
  Plan,
} from "../../bindings/github.com/yarkovaleksei/ai-memory/internal/writer";
export type { FactInput } from "../../bindings/github.com/yarkovaleksei/ai-memory/internal/adapter/claude";
export type {
  Block,
  Section,
  UnifyState,
} from "../../bindings/github.com/yarkovaleksei/ai-memory/internal/adapter/agentfiles";
export type { Consolidation } from "../../bindings/github.com/yarkovaleksei/ai-memory/internal/adapter/codex";
export type { MergeDraft } from "../../bindings/github.com/yarkovaleksei/ai-memory/internal/llm";
export type { Settings } from "../../bindings/github.com/yarkovaleksei/ai-memory/internal/config";
export type { Stats } from "../../bindings/github.com/yarkovaleksei/ai-memory/internal/library";
export {
  Install as UpdateInstall,
  State as UpdateState,
} from "../../bindings/github.com/yarkovaleksei/ai-memory/internal/updates";
export type { Status as UpdateView } from "../../bindings/github.com/yarkovaleksei/ai-memory/internal/updates";
export { IndexUpdated, UpdateStatus } from "../../bindings/events";
export { Browser, Events } from "@wailsio/runtime";
