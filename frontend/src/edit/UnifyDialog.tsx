import {
  Alert,
  Box,
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Stack,
  Typography,
} from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslate } from "react-admin";
import { PlansService, type Block, type Item, type UnifyState } from "../api/bindings";
import { call } from "../api/errors";
import { useRunPlan } from "./plans";

// Состояния, в которых приём имеет смысл (agentfiles.Unify*).
const OFFER = new Set(["both", "claude-only", "agents-only"]);

// Предложение показывается у главного файла инструкций проекта: там, где
// пользователь видит, что правила двух агентов расходятся.
const isProjectMain = (it: Item) =>
  it.kind === "instructions" && it.role === "main" && it.projectId !== "global";

// По умолчанию в AGENTS.md уходит то, что похоже на общие правила: не
// заголовок документа, без импортов @ (Codex их не поймёт) и не раздел,
// который в заголовке называет Claude.
export const sharedByDefault = (b: Block) =>
  !b.intro && (b.imports ?? []).length === 0 && !/claude/i.test(b.heading);

// UnifyHint — предложение приёма «AGENTS.md — основной, CLAUDE.md
// импортирует его» (ADR-022) с кнопкой, открывающей диалог.
export const UnifyHint = ({ item, disabled }: { item: Item; disabled: boolean }) => {
  const translate = useTranslate();
  const [open, setOpen] = useState(false);
  const enabled = isProjectMain(item);
  const { data } = useQuery<UnifyState>({
    queryKey: ["plans", "unify", item.projectId, item.hash],
    queryFn: () => call(() => PlansService.UnifyDraft(item.projectId)),
    enabled,
    retry: false,
  });
  if (!enabled || !data || !OFFER.has(data.state)) return null;
  return (
    <>
      <Alert
        severity="info"
        action={
          <Button color="inherit" size="small" disabled={disabled} onClick={() => setOpen(true)}>
            {translate("aim.unify.action")}
          </Button>
        }
      >
        {translate(`aim.unify.hint.${data.state}`)}
      </Alert>
      {open && <UnifyDialog projectId={item.projectId} onClose={() => setOpen(false)} />}
    </>
  );
};

const UnifyDialog = ({ projectId, onClose }: { projectId: string; onClose: () => void }) => {
  const translate = useTranslate();
  // Диалог перечитывает состояние: хэши должны быть свежими.
  const { data } = useQuery<UnifyState>({
    queryKey: ["plans", "unify-dialog", projectId],
    queryFn: () => call(() => PlansService.UnifyDraft(projectId)),
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
  return (
    <Dialog open onClose={onClose} maxWidth="md" fullWidth aria-labelledby="unify-title">
      <DialogTitle id="unify-title">{translate("aim.unify.title")}</DialogTitle>
      {data ? (
        <UnifyForm projectId={projectId} state={data} onClose={onClose} />
      ) : (
        <DialogContent>
          <Typography color="text.secondary">{translate("aim.preview.loading")}</Typography>
        </DialogContent>
      )}
    </Dialog>
  );
};

const UnifyForm = ({
  projectId,
  state,
  onClose,
}: {
  projectId: string;
  state: UnifyState;
  onClose: () => void;
}) => {
  const translate = useTranslate();
  const runPlan = useRunPlan();
  const blocks = state.blocks ?? [];
  const [move, setMove] = useState<Set<number>>(
    () => new Set(blocks.filter(sharedByDefault).map((b) => b.index)),
  );
  const toggle = (i: number) =>
    setMove((m) => {
      const next = new Set(m);
      if (next.has(i)) next.delete(i);
      else next.add(i);
      return next;
    });
  const empty = state.state === "claude-only" && move.size === 0;
  return (
    <>
      <DialogContent dividers>
        <Stack spacing={2}>
          <Typography>{translate(`aim.unify.help.${state.state}`, { import: state.import })}</Typography>
          {state.override && <Alert severity="warning">{translate("aim.unify.override")}</Alert>}
          {blocks.length > 0 && (
            <Stack spacing={1}>
              <Typography variant="subtitle2">{translate("aim.unify.blocks")}</Typography>
              {blocks.map((b) => (
                <Box key={b.index}>
                  <FormControlLabel
                    control={<Checkbox checked={move.has(b.index)} onChange={() => toggle(b.index)} />}
                    label={b.heading || translate("aim.section.preamble")}
                  />
                  <Box
                    component="pre"
                    sx={{
                      m: 0,
                      ml: 4,
                      p: 1,
                      maxHeight: 120,
                      overflow: "auto",
                      bgcolor: "action.hover",
                      borderRadius: 1,
                      fontFamily: "monospace",
                      fontSize: 12,
                      whiteSpace: "pre-wrap",
                      wordBreak: "break-word",
                    }}
                  >
                    {b.text}
                  </Box>
                  {(b.imports ?? []).length > 0 && move.has(b.index) && (
                    <Typography variant="caption" color="warning.main" sx={{ ml: 4 }}>
                      {translate("aim.section.imports", { imports: (b.imports ?? []).join(", ") })}
                    </Typography>
                  )}
                </Box>
              ))}
            </Stack>
          )}
          {empty && <Alert severity="info">{translate("aim.unify.empty")}</Alert>}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{translate("ra.action.cancel")}</Button>
        <Button
          variant="contained"
          disabled={empty}
          onClick={() =>
            void runPlan(
              () =>
                PlansService.Unify(
                  projectId,
                  state.claudeHash,
                  state.agentsHash,
                  [...move].sort((a, b) => a - b),
                ),
              { onApplied: onClose },
            )
          }
        >
          {translate("aim.plan.preview")}
        </Button>
      </DialogActions>
    </>
  );
};
