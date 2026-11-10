import RefreshIcon from "@mui/icons-material/Refresh";
import { Box, IconButton, Paper, Stack, Tooltip, Typography } from "@mui/material";
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
import { BulkDeleteFacts, CreateMenu, isSelectableFact } from "../edit/ItemActions";
import { RELEVANCE, type AimDataProvider } from "../dataProvider";
import { AgentBadge, TypeBadge } from "./badges";
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
    sx={{ minWidth: 150 }}
    source="agent"
    alwaysOn
    choices={[
      { id: "claude", name: "aim.agents.claude" },
      { id: "codex", name: "aim.agents.codex" },
    ]}
  />,
  <SelectInput
    key="type"
    fullWidth={false}
    sx={{ minWidth: 150 }}
    source="type"
    alwaysOn
    choices={["user", "feedback", "project", "reference"].map((t) => ({ id: t, name: `aim.types.${t}` }))}
  />,
  <BooleanInput key="includeTemp" source="includeTemp" alwaysOn />,
  <BooleanInput key="warningsOnly" source="warningsOnly" alwaysOn />,
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
        <Stack direction="row" sx={{ alignItems: "flex-start", px: 1 }}>
          <Box sx={{ flex: 1, minWidth: 0, "& .RaFilterFormInput-body": { mr: 1 } }}>
            <FilterForm filters={itemFilters} />
          </Box>
          <Stack direction="row" sx={{ pt: 1.5 }}>
            <CreateMenu />
            <RebuildButton />
          </Stack>
        </Stack>
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
  const { total, isPending } = useListContext<Row>();
  if (!isPending && total === 0) {
    return (
      <Box sx={{ p: 3 }}>
        <Typography>{translate("aim.list.empty")}</Typography>
        <Typography variant="body2" color="text.secondary">
          {translate("aim.list.emptyHint")}
        </Typography>
      </Box>
    );
  }
  return (
    <DataTable<Row>
      bulkActionButtons={<BulkDeleteFacts />}
      isRowSelectable={isSelectableFact}
      rowClick={(id) => {
        onSelect(String(id));
        return false;
      }}
      rowSx={(record) => (record.id === selected ? { bgcolor: "action.selected" } : {})}
      size="small"
    >
      <DataTable.Col source="title" render={() => <TitleCell />} />
      <DataTable.Col source="agent" render={(r) => <AgentBadge agent={r.agent} />} />
      <DataTable.Col source="type" render={(r) => <TypeBadge type={r.type} />} />
      <DataTable.Col source="modTime" field={DateField} />
    </DataTable>
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
  // Иконка, а не кнопка с подписью: в средней колонке подпись выталкивает фильтры в столбик.
  return (
    <Tooltip title={translate("aim.list.rebuild")}>
      <span>
        <IconButton aria-label={translate("aim.list.rebuild")} onClick={() => mutate()} disabled={isPending}>
          <RefreshIcon />
        </IconButton>
      </span>
    </Tooltip>
  );
};
