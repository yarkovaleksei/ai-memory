import { useGetList } from "react-admin";
import type { Source } from "../api/bindings";

export interface WritableAgents {
  claude: boolean;
  codex: boolean;
}

// useWritableAgents — в память каких агентов можно писать: есть хранилище
// агента и оно не только для чтения. Нет каталога Claude или Codex — нет и
// действий, которые в него пишут (создать, перенести, итог объединения): иначе
// кнопка вела бы к ошибке. Пока источники не загружены, ничего не предлагаем.
export const useWritableAgents = (): WritableAgents => {
  const { data = [] } = useGetList<Source & { id: string }>("sources", {
    pagination: { page: 1, perPage: 1000 },
  });
  const has = (agent: string) => data.some((s) => s.agent === agent && !s.readOnly);
  return { claude: has("claude"), codex: has("codex") };
};
