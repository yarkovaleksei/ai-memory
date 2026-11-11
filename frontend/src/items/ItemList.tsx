import CloseIcon from "@mui/icons-material/Close";
import RefreshIcon from "@mui/icons-material/Refresh";
import SearchOffIcon from "@mui/icons-material/SearchOff";
import { alpha, Box, Button, IconButton, Paper, Stack, Tooltip, Typography } from "@mui/material";
import { useMutation } from "@tanstack/react-query";
import { useCallback, useState } from "react";
import {
  BooleanInput,
  DataTable,
  DateField,
  FilterForm,
  ListBase,
  Pagination,
  SearchInput,
  SelectInput,
  Title,
  useDataProvider,
  useListContext,
  useNotify,
  useRecordContext,
  useRefresh,
  useTranslate,
} from "react-admin";
import type { ListItem } from "../api/bindings";
import { BulkActions, CreateMenu, isSelectable } from "../edit/ItemActions";
import { RELEVANCE, type AimDataProvider } from "../dataProvider";
import { EmptyState } from "../layout/EmptyState";
import { AgentBadge, KindBadge } from "./badges";
import { ItemPreview, PendingChip } from "./ItemPreview";
import { ProjectTree } from "./ProjectTree";
import { splitSnippet, useErrorText } from "./text";

type Row = ListItem & { id: string };

// Названия вариантов — ключи словаря: SelectInput переводит их сам.
const itemFilters = [
  <SearchInput key="q" source="q" alwaysOn fullWidth={false} />,
  <SelectInput
    key="agent"
    fullWidth={false}
    source="agent"
    alwaysOn
    choices={[
      { id: "claude", name: "aim.agents.claude" },
      { id: "codex", name: "aim.agents.codex" },
    ]}
  />,
  // Раздел: вся автопамять, инструкции или навыки. «memory» — фильтр индекса
  // «всё, кроме инструкций и навыков».
  <SelectInput
    key="kind"
    fullWidth={false}
    source="kind"
    alwaysOn
    choices={["memory", "instructions", "skill"].map((k) => ({ id: k, name: `aim.sections.${k}` }))}
  />,
  <SelectInput
    key="type"
    fullWidth={false}
    source="type"
    alwaysOn
    choices={["user", "feedback", "project", "reference"].map((t) => ({ id: t, name: `aim.types.${t}` }))}
  />,
  // size="medium" возвращает переключателям штатный размер: FilterForm
  // react-admin делает все фильтры маленькими, поля же и так small по теме.
  <BooleanInput key="includeTemp" source="includeTemp" alwaysOn size="medium" />,
  <BooleanInput key="warningsOnly" source="warningsOnly" alwaysOn size="medium" />,
];

// Главный экран: дерево проектов | список записей | просмотр записи.
export const ItemList = () => (
  <ListBase<Row> resource="items" sort={{ field: RELEVANCE, order: "DESC" }} perPage={50}>
    <ItemListView />
  </ListBase>
);

const ItemListView = () => {
  const translate = useTranslate();
  const [selected, setSelected] = useState<string | null>(null);
  const open = useCallback((id: string) => setSelected(id), []);

  return (
    <Box
      sx={{
        display: "grid",
        gridTemplateColumns: "240px minmax(0, 1.4fr) minmax(0, 1fr)",
        gap: 1,
        height: "calc(100vh - 64px)",
        py: 1,
      }}
    >
      <Title title={translate("aim.menu.memory")} />
      <Paper variant="outlined" sx={{ overflow: "hidden" }}>
        <ProjectTree />
      </Paper>
      <Paper variant="outlined" sx={{ display: "flex", flexDirection: "column", minHeight: 0 }}>
        {/* В узкой колонке (окно минимальной ширины) кнопки переносятся на
            строку ниже фильтров, а не наезжают на поле поиска. */}
        <Stack direction="row" sx={{ alignItems: "flex-start", flexWrap: "wrap", columnGap: 1, px: 1 }}>
          {/* Поля фильтров — одной ширины; высоту и вид задаёт тема. */}
          <Box
            sx={{
              flex: "1 1 220px",
              minWidth: 0,
              "& .RaFilterForm-filterFormInput": { mr: 1 },
              "& .RaFilterForm-filterFormInput .MuiInputBase-root": { width: 200, maxWidth: "100%" },
            }}
          >
            <FilterForm filters={itemFilters} />
          </Box>
          {/* Отступ сверху — как у полей фильтров (margin dense): кнопки на одной линии с ними. */}
          <Stack direction="row" spacing={1} sx={{ mt: 1, mb: 1, flexShrink: 0, ml: "auto" }}>
            <CreateMenu />
            <RebuildButton />
          </Stack>
        </Stack>
        <SelectionBar />
        <Box sx={{ flex: 1, overflowY: "auto", minHeight: 0 }}>
          <ItemTable selected={selected} onSelect={open} />
        </Box>
        <Pagination rowsPerPageOptions={[25, 50, 100, 200]} />
      </Paper>
      <Paper variant="outlined" sx={{ overflowY: "auto" }}>
        <ItemPreview id={selected} onOpenItem={open} />
      </Paper>
    </Box>
  );
};

const ItemTable = ({ selected, onSelect }: { selected: string | null; onSelect: (id: string) => void }) => {
  const translate = useTranslate();
  const { data, total, isPending } = useListContext<Row>();
  // Отметки — для действий над несколькими записями. Нет на странице записей,
  // которые можно отметить, — нет и колонки; у остальных строк отметка скрыта,
  // а не серая.
  const selectable = (data ?? []).some(isSelectable);
  if (!isPending && total === 0) {
    return (
      <EmptyState
        icon={SearchOffIcon}
        title={translate("aim.list.empty")}
        hint={translate("aim.list.emptyHint")}
      />
    );
  }
  return (
    <DataTable<Row>
      // Панель действий над отмеченными — своя, SelectionBar над таблицей:
      // штатная react-admin висит над таблицей со сдвигом вверх, и прокручиваемый
      // блок вокруг таблицы её обрезает. bulkActionButtons нужен только для
      // колонки отметок.
      bulkActionButtons={selectable}
      bulkActionsToolbar={false}
      isRowSelectable={isSelectable}
      rowClick={(id) => {
        onSelect(String(id));
        return false;
      }}
      rowSx={(record) => (record.id === selected ? { bgcolor: "action.selected" } : {})}
      size="small"
      sx={{ "& .RaDataTable-checkbox.Mui-disabled": { visibility: "hidden" } }}
    >
      <DataTable.Col source="title" render={() => <TitleCell />} />
      <DataTable.Col source="agent" render={(r) => <AgentBadge agent={r.agent} />} />
      <DataTable.Col source="type" render={(r) => <KindBadge kind={r.kind} type={r.type} />} />
      <DataTable.Col source="modTime" field={DateField} />
    </DataTable>
  );
};

// SelectionBar — действия над отмеченными записями. Видна, пока есть отметки.
const SelectionBar = () => {
  const translate = useTranslate();
  const { selectedIds, onUnselectItems } = useListContext();
  if (selectedIds.length === 0) return null;
  return (
    <Stack
      direction="row"
      spacing={1}
      sx={{
        alignItems: "center",
        px: 1,
        py: 1,
        flexShrink: 0,
        bgcolor: (theme) => alpha(theme.palette.primary.main, 0.08),
      }}
    >
      <Tooltip title={translate("aim.list.unselect")}>
        <IconButton aria-label={translate("aim.list.unselect")} onClick={() => onUnselectItems()}>
          <CloseIcon />
        </IconButton>
      </Tooltip>
      <Typography sx={{ flex: 1 }}>
        {translate("aim.list.selected", { smart_count: selectedIds.length })}
      </Typography>
      <BulkActions />
    </Stack>
  );
};

// Название записи и под ним сниппет совпадения с подсветкой.
const TitleCell = () => {
  const record = useRecordContext<Row>();
  if (!record) return null;
  return (
    <Box sx={{ minWidth: 0 }}>
      <Typography variant="body2" sx={{ wordBreak: "break-word" }}>
        {record.title}
      </Typography>
      {record.pending && <PendingChip />}
      {record.snippet && (
        <Typography variant="caption" color="text.secondary" component="div" sx={{ wordBreak: "break-word" }}>
          {splitSnippet(record.snippet).map((p, i) =>
            p.hit ? (
              <Box key={i} component="mark" sx={{ bgcolor: "warning.light", color: "inherit", px: 0.25 }}>
                {p.text}
              </Box>
            ) : (
              <span key={i}>{p.text}</span>
            ),
          )}
        </Typography>
      )}
    </Box>
  );
};

const RebuildButton = () => {
  const dataProvider = useDataProvider<AimDataProvider>();
  const notify = useNotify();
  const refresh = useRefresh();
  const errorText = useErrorText();
  const { mutate, isPending } = useMutation({
    mutationFn: () => dataProvider.rebuild(),
    onSuccess: (st) => {
      notify("aim.list.rebuilt", { type: "info", messageArgs: { items: st.items, duration: st.duration } });
      refresh();
    },
    onError: (err) => notify(errorText(err), { type: "error" }),
  });
  const translate = useTranslate();
  return (
    <Button variant="outlined" startIcon={<RefreshIcon />} onClick={() => mutate()} disabled={isPending}>
      {translate("aim.list.rebuild")}
    </Button>
  );
};
