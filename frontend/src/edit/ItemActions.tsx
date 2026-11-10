import AddIcon from "@mui/icons-material/Add";
import MergeIcon from "@mui/icons-material/CallMerge";
import DeleteIcon from "@mui/icons-material/DeleteOutlined";
import ContradictionsIcon from "@mui/icons-material/RuleOutlined";
import { Button, Menu, MenuItem, Stack, Tooltip } from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
import { useListContext, useTranslate } from "react-admin";
import { LibraryService, PlansService, type Item } from "../api/bindings";
import { call } from "../api/errors";
import { ContradictionsDialog } from "./ContradictionsDialog";
import { FactEditor, type FactTarget } from "./FactEditor";
import { MergeDialog } from "./MergeDialog";
import { NoteEditor, type NoteTarget } from "./NoteEditor";
import { useWritableAgents } from "./agents";
import { TransferDialog, transferTargets } from "./TransferDialog";
import { useEditStatus, useRunPlan } from "./plans";

// useWriteGate — запись выключена (нет git для бэкапов): кнопки видны, но
// неактивны, и подсказка объясняет почему.
const useWriteGate = () => {
  const translate = useTranslate();
  const { data } = useEditStatus();
  const disabled = !!data && !data.enabled;
  return { disabled, reason: disabled ? translate(`aim.errors.${data.reason}`, { _: data.reason }) : "" };
};

const Gate = ({ reason, children }: { reason: string; children: ReactNode }) =>
  reason ? (
    <Tooltip title={reason}>
      <span>{children}</span>
    </Tooltip>
  ) : (
    <>{children}</>
  );

const isFact = (it: Pick<Item, "kind" | "write">) => it.kind === "claude-fact" && it.write === "direct";

// Инструкции и навыки — не автопамять: перенос факта, объединение, «Забыть»
// к ним не применяются, у них будут свои операции (ADR-019).
export const isMemory = (it: Pick<Item, "kind">) => it.kind !== "instructions" && it.kind !== "skill";

// ItemActions — что можно сделать с записью. Набор зависит от вида записи:
// факт Claude правится и удаляется напрямую, записи Codex меняются только
// заметками (ADR-002), MEMORY.md чинится по файлам фактов.
export const ItemActions = ({ item }: { item: Item }) => {
  const translate = useTranslate();
  const runPlan = useRunPlan();
  const { disabled, reason } = useWriteGate();
  const [fact, setFact] = useState<FactTarget | null>(null);
  const [note, setNote] = useState<NoteTarget | null>(null);
  const [transfer, setTransfer] = useState(false);
  const agents = useWritableAgents();

  const indexIssue = (item.warnings ?? []).some((w) => w.code.startsWith("claude.index."));
  const buttons: ReactNode[] = [];
  const add = (key: string, label: string, onClick: () => void, color?: "error") =>
    buttons.push(
      <Button key={key} variant="outlined" color={color} disabled={disabled} onClick={onClick}>
        {translate(label)}
      </Button>,
    );

  if (isFact(item)) {
    add("edit", "aim.actions.edit", () => setFact({ kind: "edit", itemId: item.id }));
    add(
      "delete",
      "aim.actions.delete",
      () => void runPlan(() => PlansService.DeleteFacts([item.id])),
      "error",
    );
  }
  // Только когда индексу есть что исправить: строки без файла или факты вне
  // индекса. Иначе кнопка вела бы к «Изменений нет».
  if (item.agent === "claude" && indexIssue) {
    add("fix", "aim.actions.fixIndex", () => void runPlan(() => PlansService.FixIndex(item.id)));
  }
  if (item.kind === "codex-adhoc-note" && item.write === "direct") {
    add("editNote", "aim.actions.editNote", () => setNote({ kind: "edit", itemId: item.id }));
  }
  if (item.agent === "codex" && item.write !== "none" && isMemory(item)) {
    add("refine", "aim.actions.refine", () => setNote({ kind: "refine", title: item.title }));
    add("forget", "aim.actions.forget", () => setNote({ kind: "forget", title: item.title }), "error");
  }
  if (item.kind !== "claude-index" && isMemory(item) && transferTargets(item, agents).length > 0) {
    add("transfer", "aim.actions.transfer", () => setTransfer(true));
  }
  if (buttons.length === 0) return null;

  return (
    <>
      <Gate reason={reason}>
        <Stack direction="row" spacing={1} sx={{ flexWrap: "wrap", rowGap: 1 }}>
          {buttons}
        </Stack>
      </Gate>
      {fact && <FactEditor target={fact} onClose={() => setFact(null)} />}
      {note && <NoteEditor target={note} onClose={() => setNote(null)} />}
      {transfer && <TransferDialog item={item} onClose={() => setTransfer(false)} />}
    </>
  );
};

// CreateMenu — новый факт Claude в выбранном проекте или новая заметка Codex.
export const CreateMenu = () => {
  const translate = useTranslate();
  const { filterValues } = useListContext();
  const { disabled, reason } = useWriteGate();
  const agents = useWritableAgents();
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const [fact, setFact] = useState<FactTarget | null>(null);
  const [note, setNote] = useState<NoteTarget | null>(null);
  const projectId = (filterValues as { projectId?: string }).projectId;
  const pick = (fn: () => void) => () => {
    setAnchor(null);
    fn();
  };
  // Писать некуда — ни Claude, ни Codex: создавать нечего.
  if (!agents.claude && !agents.codex) return null;
  return (
    <>
      {/* Создание — главное действие экрана: акцентная кнопка с подписью. */}
      <Tooltip title={reason}>
        <span>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            disabled={disabled}
            onClick={(e) => setAnchor(e.currentTarget)}
            sx={{ whiteSpace: "nowrap" }}
          >
            {translate("aim.actions.create")}
          </Button>
        </span>
      </Tooltip>
      <Menu anchorEl={anchor} open={!!anchor} onClose={() => setAnchor(null)}>
        {agents.claude && (
          <MenuItem onClick={pick(() => setFact({ kind: "create", projectId }))}>
            {translate("aim.actions.createFact")}
          </MenuItem>
        )}
        {agents.codex && (
          <MenuItem onClick={pick(() => setNote({ kind: "create" }))}>
            {translate("aim.actions.createNote")}
          </MenuItem>
        )}
      </Menu>
      {fact && <FactEditor target={fact} onClose={() => setFact(null)} />}
      {note && <NoteEditor target={note} onClose={() => setNote(null)} />}
    </>
  );
};

// useSelectedItems — отмеченные в списке записи целиком: в строках списка нет
// текста, а отметки могут быть на разных страницах.
const useSelectedItems = (ids: string[]) =>
  useQuery<Item[]>({
    queryKey: ["items", "selected", ids],
    queryFn: async () => (await call(() => LibraryService.GetItems(ids))) ?? [],
    enabled: ids.length > 0,
  });

// BulkActions — действия над отмеченными записями: объединить (с черновиком
// от модели), найти противоречия между ними, удалить факты Claude.
export const BulkActions = () => {
  const translate = useTranslate();
  const runPlan = useRunPlan();
  const { disabled, reason } = useWriteGate();
  const { selectedIds, onUnselectItems } = useListContext();
  const agents = useWritableAgents();
  const ids = selectedIds.map(String);
  const { data: items = [] } = useSelectedItems(ids);
  const ready = items.length === ids.length && ids.length > 0;
  const [dialog, setDialog] = useState<"merge" | "contradictions" | null>(null);
  const onlyFacts = ready && items.every(isFact);
  return (
    <Stack direction="row" spacing={1}>
      <Gate reason={reason}>
        <Button
          variant="outlined"
          startIcon={<MergeIcon />}
          disabled={disabled || !ready || ids.length < 2 || (!agents.claude && !agents.codex)}
          onClick={() => setDialog("merge")}
        >
          {translate("aim.actions.merge")}
        </Button>
      </Gate>
      <Button
        variant="outlined"
        startIcon={<ContradictionsIcon />}
        disabled={!ready || ids.length < 2}
        onClick={() => setDialog("contradictions")}
      >
        {translate("aim.actions.contradictions")}
      </Button>
      <Gate reason={reason || (ready && !onlyFacts ? translate("aim.actions.deleteOnlyFacts") : "")}>
        <Button
          variant="outlined"
          color="error"
          startIcon={<DeleteIcon />}
          disabled={disabled || !onlyFacts}
          onClick={() =>
            void runPlan(() => PlansService.DeleteFacts(ids), {
              onApplied: () => onUnselectItems(),
            })
          }
        >
          {translate("aim.actions.deleteSelected", { smart_count: ids.length })}
        </Button>
      </Gate>
      {dialog === "merge" && (
        <MergeDialog items={items} onClose={() => setDialog(null)} onApplied={() => onUnselectItems()} />
      )}
      {dialog === "contradictions" && <ContradictionsDialog items={items} onClose={() => setDialog(null)} />}
    </Stack>
  );
};

// Отмечать можно то, что объединение умеет убрать: факт Claude удаляется,
// для записи Codex пишется заметка «Забыть». Сводки и сгенерированные файлы
// Codex только читаются.
export const isSelectable = (it: Pick<Item, "agent" | "kind" | "write">) =>
  isFact(it) || (it.agent === "codex" && it.write !== "none" && isMemory(it));
