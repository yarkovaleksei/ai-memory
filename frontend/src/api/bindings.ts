// Единственная точка импорта сгенерированных привязок Wails и рантайма: длинный путь
// пакета Go живёт только здесь, а тесты подменяют этот модуль целиком.
export { InfoService, LibraryService } from "../../bindings/github.com/yarkovaleksei/ai-memory/internal/app";
export type {
  AppInfo,
  ItemPage,
  ItemQuery,
  ListItem,
} from "../../bindings/github.com/yarkovaleksei/ai-memory/internal/app";
export type {
  Item,
  Link,
  Project,
  Source,
  Warning,
} from "../../bindings/github.com/yarkovaleksei/ai-memory/internal/model";
export type { Stats } from "../../bindings/github.com/yarkovaleksei/ai-memory/internal/library";
export { IndexUpdated } from "../../bindings/events";
export { Browser, Events } from "@wailsio/runtime";
