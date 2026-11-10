import {
  Alert,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Stack,
  Typography,
} from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import { useNotify, useRefresh, useTranslate } from "react-admin";
import {
  PlansService,
  type ApplyResult,
  type Change,
  type EditStatus,
  type Op,
  type Plan,
} from "../api/bindings";
import { call } from "../api/errors";
import { useErrorText } from "../items/text";
import { DiffView } from "./DiffView";

// Запись в два шага (plan/architecture.md): функция строит план в Go, диалог
// показывает diff каждого файла, и только «Записать» применяет план по его ID.

// useOpText — текст операции по коду и параметрам из Go.
export const useOpText = () => {
  const translate = useTranslate();
  return useCallback(
    (op: Op) => {
      const params = op.params ?? {};
      if (op.code === "revert") {
        const what = translate(`aim.opKinds.${params.op}`, { _: params.op ?? "" });
        return translate("aim.ops.revert", { op: what });
      }
      return translate(`aim.ops.${op.code}`, { ...params, _: op.code });
    },
    [translate],
  );
};

export const EDIT_STATUS_KEY = ["plans", "status"];

// useEditStatus — включена ли запись и что известно о консолидации Codex.
// Обновляется вместе со всем остальным по useRefresh.
export const useEditStatus = () =>
  useQuery<EditStatus>({
    queryKey: EDIT_STATUS_KEY,
    queryFn: () => call(() => PlansService.Status()),
  });

// useConsolidationHint — когда Codex учтёт заметку (storage-formats.md: первый
// ход любой сессии после конца прошлой консолидации + 6 часов).
export const useConsolidationHint = () => {
  const translate = useTranslate();
  // «Сейчас» — момент, когда статус получен: так подсказка не меняется от
  // перерисовки к перерисовке, а свежеет вместе со статусом.
  const { data, dataUpdatedAt } = useEditStatus();
  const c = data?.codex;
  if (!c) return translate("aim.codex.hint.unknown", { hours: 6 });
  const cons = c.consolidation;
  if (cons.status === "running") return translate("aim.codex.hint.running");
  if (cons.notBefore && new Date(cons.notBefore).getTime() > dataUpdatedAt) {
    return translate("aim.codex.hint.after", { time: new Date(cons.notBefore).toLocaleString() });
  }
  if (cons.known) return translate("aim.codex.hint.next");
  return translate("aim.codex.hint.unknown", { hours: c.cooldownHours });
};

export interface RunOptions {
  onApplied?: (res: ApplyResult) => void;
}

type RunPlan = (build: () => Promise<Plan>, opts?: RunOptions) => Promise<boolean>;

const PlanContext = createContext<RunPlan | null>(null);

// useRunPlan строит план и открывает диалог подтверждения. Вернёт false, если
// план не построился (ошибка уже показана уведомлением).
export const useRunPlan = (): RunPlan => {
  const run = useContext(PlanContext);
  if (!run) throw new Error("useRunPlan вне PlanProvider");
  return run;
};

export const PlanProvider = ({ children }: { children: ReactNode }) => {
  const notify = useNotify();
  const refresh = useRefresh();
  const errorText = useErrorText();
  const [pending, setPending] = useState<{ plan: Plan; opts: RunOptions } | null>(null);
  const [applying, setApplying] = useState(false);

  const run = useCallback<RunPlan>(
    async (build, opts = {}) => {
      try {
        const plan = await call(build);
        setPending({ plan, opts });
        return true;
      } catch (err) {
        notify(errorText(err), { type: "error", multiLine: true });
        return false;
      }
    },
    [notify, errorText],
  );

  const cancel = () => {
    if (pending) void PlansService.Discard(pending.plan.id).catch(() => undefined);
    setPending(null);
  };

  const apply = async () => {
    if (!pending) return;
    setApplying(true);
    try {
      const res = await call(() => PlansService.Apply(pending.plan.id));
      notify("aim.plan.applied", { type: "success" });
      pending.opts.onApplied?.(res);
    } catch (err) {
      // План одноразовый: после отказа его строят заново, поэтому диалог закрывается.
      notify(errorText(err), { type: "error", multiLine: true });
    } finally {
      setApplying(false);
      setPending(null);
      refresh();
    }
  };

  return (
    <PlanContext.Provider value={run}>
      {children}
      {pending && <PlanDialog plan={pending.plan} busy={applying} onCancel={cancel} onApply={apply} />}
    </PlanContext.Provider>
  );
};

interface PlanDialogProps {
  plan: Plan;
  busy: boolean;
  onCancel: () => void;
  onApply: () => void;
}

const PlanDialog = ({ plan, busy, onCancel, onApply }: PlanDialogProps) => {
  const translate = useTranslate();
  const opText = useOpText();
  const hint = useConsolidationHint();
  const changes = useMemo(() => plan.changes ?? [], [plan]);
  // Заметки Codex — единственное, что ai-memory пишет в Codex (ADR-002): по их
  // каталогу видно, что изменение вступит только после консолидации.
  const touchesCodex = changes.some((c) => /[\\/]ad_hoc[\\/]notes[\\/]/.test(c.path));
  return (
    <Dialog open onClose={busy ? undefined : onCancel} maxWidth="lg" fullWidth aria-labelledby="plan-title">
      <DialogTitle id="plan-title">{opText(plan.op)}</DialogTitle>
      <DialogContent dividers>
        <Stack spacing={2}>
          <Typography color="text.secondary">
            {translate("aim.plan.intro", { smart_count: changes.length })}
          </Typography>
          {touchesCodex && <Alert severity="info">{hint}</Alert>}
          {changes.map((c) => (
            <Stack key={c.path} spacing={0.5}>
              <Stack direction="row" spacing={1} sx={{ alignItems: "center", minWidth: 0 }}>
                <ChangeChip change={c} />
                <Typography variant="body2" sx={{ fontFamily: "monospace", wordBreak: "break-all" }}>
                  {c.path}
                </Typography>
              </Stack>
              <DiffView change={c} />
            </Stack>
          ))}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onCancel} disabled={busy}>
          {translate("ra.action.cancel")}
        </Button>
        <Button variant="contained" onClick={onApply} disabled={busy}>
          {translate("aim.plan.apply")}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

const ChangeChip = ({ change }: { change: Change }) => {
  const translate = useTranslate();
  if (!change.beforeExists)
    return <Chip size="small" color="success" label={translate("aim.plan.created")} />;
  if (!change.afterExists) return <Chip size="small" color="error" label={translate("aim.plan.deleted")} />;
  return <Chip size="small" variant="outlined" label={translate("aim.plan.changed")} />;
};
