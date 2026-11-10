import type { DataProvider, GetListParams, RaRecord } from "react-admin";
import { LibraryService, type ItemQuery, type Stats } from "./api/bindings";
import { AppError, call } from "./api/errors";

// Ресурсы — только чтение:
//  - items    — записи памяти; фильтры, поиск, сортировка и страницы считает Go;
//  - projects — проекты в порядке показа (уже отсортированы Go);
//  - sources  — хранилища памяти.
// Пишущие методы отклоняются: запись идёт не через CRUD react-admin, а планом с
// diff (PlansService, src/edit/plans.tsx), чтобы ничего не писалось без подтверждения.

// Поле сортировки «по релевантности»: Go сортирует по рангу при поиске и по дате без него.
export const RELEVANCE = "relevance";

const SORTABLE = new Set(["title", "modTime", "agent", "type"]);

type Filter = Partial<Pick<ItemQuery, "projectId" | "agent" | "kind" | "type" | "q">> & {
  includeTemp?: boolean;
  warningsOnly?: boolean;
};

export const toItemQuery = (params: GetListParams): ItemQuery => {
  const filter = (params.filter ?? {}) as Filter;
  const page = params.pagination?.page ?? 1;
  const perPage = params.pagination?.perPage ?? 50;
  const field = params.sort?.field ?? RELEVANCE;
  return {
    projectId: filter.projectId ?? "",
    agent: filter.agent ?? "",
    kind: filter.kind ?? "",
    type: filter.type ?? "",
    q: filter.q ?? "",
    includeTemp: Boolean(filter.includeTemp),
    warningsOnly: Boolean(filter.warningsOnly),
    sort: SORTABLE.has(field) ? field : "",
    desc: params.sort?.order === "DESC",
    offset: (page - 1) * perPage,
    limit: perPage,
  };
};

const readOnly = () => Promise.reject(new AppError("readonly"));

const allOf = async <T extends RaRecord>(fn: () => Promise<T[] | null>) => {
  const data = (await call(fn)) ?? [];
  return { data, total: data.length };
};

const oneOf = async <T extends RaRecord>(fn: () => Promise<T[] | null>, id: unknown) => {
  const found = ((await call(fn)) ?? []).find((r) => r.id === id);
  if (!found) throw new AppError("item.not-found");
  return { data: found };
};

const unknownResource = (resource: string) => Promise.reject(new AppError("resource.unknown", { resource }));

export interface AimDataProvider extends DataProvider {
  rebuild: () => Promise<Stats>;
}

// Привязки возвращают точные типы моделей Go, react-admin ждёт RaRecord: приведение
// через unknown безопасно, у всех трёх моделей есть строковый id.
export const dataProvider: AimDataProvider = {
  getList: async (resource, params) => {
    switch (resource) {
      case "items": {
        const page = await call(() => LibraryService.ListItems(toItemQuery(params)));
        return { data: page.items as unknown as never[], total: page.total };
      }
      case "projects":
        return (await allOf(() => LibraryService.Projects())) as never;
      case "sources":
        return (await allOf(() => LibraryService.Sources())) as never;
      default:
        return unknownResource(resource);
    }
  },
  getOne: async (resource, { id }) => {
    switch (resource) {
      case "items":
        return { data: (await call(() => LibraryService.GetItem(String(id)))) as never };
      case "projects":
        return (await oneOf(() => LibraryService.Projects(), id)) as never;
      case "sources":
        return (await oneOf(() => LibraryService.Sources(), id)) as never;
      default:
        return unknownResource(resource);
    }
  },
  getMany: async (resource, { ids }) => {
    switch (resource) {
      case "items": {
        const data = (await call(() => LibraryService.GetItems(ids.map(String)))) ?? [];
        return { data: data as never[] };
      }
      case "projects":
      case "sources": {
        const all = (await dataProvider.getList(resource, {})).data;
        const wanted = new Set(ids);
        return { data: all.filter((r) => wanted.has(r.id)) };
      }
      default:
        return unknownResource(resource);
    }
  },
  getManyReference: readOnly,
  create: readOnly,
  update: readOnly,
  updateMany: readOnly,
  delete: readOnly,
  deleteMany: readOnly,
  rebuild: () => call(() => LibraryService.Rebuild()),
};
