import { Alert, Box, Button, Paper, Stack, Typography } from "@mui/material";
import { useState } from "react";
import { useTranslate } from "react-admin";
import {
  LLMService,
  type Agent,
  type ContradictionReport,
  type Item,
  type SimilarQuery,
} from "../api/bindings";
import { useEditStatus } from "../edit/plans";
import { LLMActions, useLLMCall } from "../llm/llm";
import { Compare } from "./Compare";
import { MergeDialog } from "./MergeDialog";

// Contradictions — отчёт «противоречия»: модель сравнивает записи внутри групп
// похожих (тех же, что на экране) и находит пары, которые утверждают разное.
// Только показывает; свести пару можно обычным «Объединить…».
export const Contradictions = ({ query }: { query: SimilarQuery }) => {
  const translate = useTranslate();
  const llm = useLLMCall<ContradictionReport>();
  const [report, setReport] = useState<ContradictionReport | null>(null);

  const check = async (agent: Agent) => {
    const r = await llm.run(agent, (a) => LLMService.Contradictions(query, a));
    if (r) setReport(r);
  };

  const list = report?.contradictions ?? [];
  return (
    <Paper variant="outlined" sx={{ p: 2 }}>
      <Stack spacing={1.5}>
        <Typography variant="h6" component="h2">
          {translate("aim.similar.contradictions.title")}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {translate("aim.similar.contradictions.help")}
        </Typography>
        <LLMActions
          label="aim.similar.contradictions.check"
          busy="aim.similar.contradictions.checking"
          running={llm.running}
          error={llm.error}
          onRun={(a) => void check(a)}
          onStop={llm.stop}
        />
        {report && report.groups === 0 && (
          <Typography>{translate("aim.similar.contradictions.noGroups")}</Typography>
        )}
        {report && report.groups > 0 && (
          <Typography variant="body2" color="text.secondary">
            {translate("aim.similar.contradictions.checked", {
              checked: report.checked,
              groups: report.groups,
            })}
          </Typography>
        )}
        {report && report.checked < report.groups && (
          <Alert severity="warning">{translate("aim.similar.contradictions.truncated")}</Alert>
        )}
        {report && report.groups > 0 && list.length === 0 && (
          <Typography>{translate("aim.similar.contradictions.none")}</Typography>
        )}
        {list.map((c) => (
          <Finding
            key={(c.items ?? []).map((it) => it.id).join("|")}
            summary={c.summary}
            items={c.items ?? []}
          />
        ))}
      </Stack>
    </Paper>
  );
};

const Finding = ({ summary, items }: { summary: string; items: Item[] }) => {
  const translate = useTranslate();
  const { data: status } = useEditStatus();
  const [merging, setMerging] = useState(false);
  return (
    <Paper variant="outlined" sx={{ p: 1.5 }}>
      <Stack spacing={1}>
        <Stack direction="row" spacing={1} sx={{ alignItems: "center" }}>
          <Typography sx={{ flex: 1, fontWeight: 500 }}>{summary}</Typography>
          <Box>
            <Button
              size="small"
              disabled={!status?.enabled || items.length < 2}
              onClick={() => setMerging(true)}
            >
              {translate("aim.similar.merge")}
            </Button>
          </Box>
        </Stack>
        <Compare items={items} />
      </Stack>
      {merging && <MergeDialog items={items} onClose={() => setMerging(false)} />}
    </Paper>
  );
};
