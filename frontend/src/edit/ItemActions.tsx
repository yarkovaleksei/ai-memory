import AddIcon from "@mui/icons-material/Add";
import DeleteIcon from "@mui/icons-material/DeleteOutlined";
import { Button, IconButton, Menu, MenuItem, Stack, Tooltip } from "@mui/material";
import { useState, type ReactNode } from "react";
import { useListContext, useTranslate } from "react-admin";
import { PlansService, type Item } from "../api/bindings";
import { FactEditor, type FactTarget } from "./FactEditor";
import { NoteEditor, type NoteTarget } from "./NoteEditor";
import { TransferDialog } from "./TransferDialog";
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

  const indexIssue = (item.warnings ?? []).some((w) => w.code.startsWith("claude.index."));
  const buttons: ReactNode[] = [];
  const add = (key: string, label: string, onClick: () => void, color?: "error") =>
    buttons.push(
      <Button key={key} size="small" variant="outlined" color={color} disabled={disabled} onClick={onClick}>
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
  if (item.agent === "claude" && (item.kind === "claude-index" || indexIssue)) {
    add("fix", "aim.actions.fixIndex", () => void runPlan(() => PlansService.FixIndex(item.id)));
  }
  if (item.kind === "codex-adhoc-note" && item.write === "direct") {
    add("editNote", "aim.actions.editNote", () => setNote({ kind: "edit", itemId: item.id }));
  }
  if (item.agent === "codex" && item.write !== "none") {
    add("refine", "aim.actions.refine", () => setNote({ kind: "refine", title: item.title }));
    add("forget", "aim.actions.forget", () => setNote({ kind: "forget", title: item.title }), "error");
  }
  if (item.kind !== "claude-index") {
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
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const [fact, setFact] = useState<FactTarget | null>(null);
  const [note, setNote] = useState<NoteTarget | null>(null);
  const projectId = (filterValues as { projectId?: string }).projectId;
  const pick = (fn: () => void) => () => {
    setAnchor(null);
    fn();
  };
  return (
    <>
      <Tooltip title={reason || translate("aim.actions.create")}>
        <span>
          <IconButton
            aria-label={translate("aim.actions.create")}
            disabled={disabled}
            onClick={(e) => setAnchor(e.currentTarget)}
          >
            <AddIcon />
          </IconButton>
        </span>
      </Tooltip>
      <Menu anchorEl={anchor} open={!!anchor} onClose={() => setAnchor(null)}>
        <MenuItem onClick={pick(() => setFact({ kind: "create", projectId }))}>
          {translate("aim.actions.createFact")}
        </MenuItem>
        <MenuItem onClick={pick(() => setNote({ kind: "create" }))}>
          {translate("aim.actions.createNote")}
        </MenuItem>
      </Menu>
      {fact && <FactEditor target={fact} onClose={() => setFact(null)} />}
      {note && <NoteEditor target={note} onClose={() => setNote(null)} />}
    </>
  );
};

// BulkDeleteFacts — удалить выбранные факты Claude одним планом и одним diff.
export const BulkDeleteFacts = () => {
  const translate = useTranslate();
  const runPlan = useRunPlan();
  const { disabled, reason } = useWriteGate();
  const { selectedIds, onUnselectItems } = useListContext();
  return (
    <Gate reason={reason}>
      <Button
        size="small"
        color="error"
        startIcon={<DeleteIcon />}
        disabled={disabled || selectedIds.length === 0}
        onClick={() =>
          void runPlan(() => PlansService.DeleteFacts(selectedIds.map(String)), {
            onApplied: () => onUnselectItems(),
          })
        }
      >
        {translate("aim.actions.deleteSelected", { smart_count: selectedIds.length })}
      </Button>
    </Gate>
  );
};

export const isSelectableFact = isFact;
