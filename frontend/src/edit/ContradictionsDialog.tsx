import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Chip,
  Paper,
  Stack,
  Tooltip,
  Typography,
} from "@mui/material";
import { useState } from "react";
import { useTranslate } from "react-admin";
import { LLMService, PlansService, type Agent, type Contradiction, type Item } from "../api/bindings";
import { AgentBadge } from "../items/badges";
import { LLMActions, useLLMCall } from "../llm/llm";
import { MergeDialog } from "./MergeDialog";
import { isMemory } from "./ItemActions";
import { useEditStatus, useRunPlan } from "./plans";

// ContradictionsDialog — проверка отмеченных записей на противоречия: модель
// получает только их и находит пары, которые утверждают разное. Ничего не
// пишет; свести пару можно обычным «Объединить».
export const ContradictionsDialog = ({ items, onClose }: { items: Item[]; onClose: () => void }) => {
  const translate = useTranslate();
  const llm = useLLMCall<Contradiction[] | null>();
  const [found, setFound] = useState<Contradiction[] | null>(null);
  const ids = items.map((it) => it.id);

  const check = async (agent: Agent) => {
    const r = await llm.run(agent, (a) => LLMService.Contradictions(ids, a));
    if (r !== undefined) setFound(r ?? []);
  };

  return (
    <Dialog open onClose={onClose} maxWidth="md" fullWidth aria-labelledby="contradictions-title">
      <DialogTitle id="contradictions-title">
        {translate("aim.contradictions.title", { smart_count: items.length })}
      </DialogTitle>
      <DialogContent dividers>
        <Stack spacing={2}>
          <Typography variant="body2" color="text.secondary">
            {translate("aim.contradictions.help")}
          </Typography>
          <LLMActions
            label="aim.contradictions.check"
            busy="aim.contradictions.checking"
            running={llm.running}
            error={llm.error}
            onRun={(a) => void check(a)}
            onStop={llm.stop}
          />
          {found && found.length === 0 && (
            <Alert severity="success">{translate("aim.contradictions.none")}</Alert>
          )}
          {(found ?? []).map((c) => (
            <Finding key={(c.items ?? []).map((it) => it.id).join("|")} finding={c} />
          ))}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{translate("aim.contradictions.close")}</Button>
      </DialogActions>
    </Dialog>
  );
};

const Finding = ({ finding }: { finding: Contradiction }) => {
  const translate = useTranslate();
  const runPlan = useRunPlan();
  const { data: status } = useEditStatus();
  const [merging, setMerging] = useState(false);
  const items = finding.items ?? [];
  // Объединение убирает записи памяти, а инструкции — нет (ADR-024): пара
  // с инструкциями чинится правкой файла или удалением лишнего факта.
  const withInstructions = items.some((it) => !isMemory(it));
  const duplicate = finding.kind === "duplicate";
  const extraFacts = duplicate
    ? items.filter((it) => isMemory(it) && it.kind === "claude-fact" && it.write === "direct")
    : [];
  return (
    <Paper variant="outlined" sx={{ p: 2 }}>
      <Stack spacing={1}>
        <Stack direction="row" spacing={1} sx={{ alignItems: "center" }}>
          <Chip
            size="small"
            variant="outlined"
            color={duplicate ? "info" : "warning"}
            label={translate(`aim.contradictions.kinds.${duplicate ? "duplicate" : "contradiction"}`)}
          />
          <Typography sx={{ fontWeight: 500 }}>{finding.summary}</Typography>
        </Stack>
        {items.map((it) => (
          <Stack key={it.id} direction="row" spacing={1} sx={{ alignItems: "center" }}>
            <AgentBadge agent={it.agent} />
            <Typography variant="body2">{it.title}</Typography>
          </Stack>
        ))}
        <Stack direction="row" spacing={1}>
          {extraFacts.map((it) => (
            <Button
              key={it.id}
              variant="outlined"
              color="error"
              disabled={!status?.enabled}
              onClick={() => void runPlan(() => PlansService.DeleteFacts([it.id]))}
            >
              {translate("aim.contradictions.deleteFact", { title: it.title })}
            </Button>
          ))}
          <Tooltip title={withInstructions ? translate("aim.actions.mergeOnlyMemory") : ""}>
            <span>
              <Button
                variant="outlined"
                disabled={!status?.enabled || items.length < 2 || withInstructions}
                onClick={() => setMerging(true)}
              >
                {translate("aim.actions.merge")}
              </Button>
            </span>
          </Tooltip>
        </Stack>
      </Stack>
      {merging && <MergeDialog items={items} onClose={() => setMerging(false)} />}
    </Paper>
  );
};
