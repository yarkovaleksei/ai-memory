import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Checkbox,
  Chip,
  FormControlLabel,
  MenuItem,
  Paper,
  Stack,
  Switch,
  TextField,
  Typography,
} from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Title, useGetList, useTranslate } from "react-admin";
import { LibraryService, type Project, type SimilarGroup } from "../api/bindings";
import { call } from "../api/errors";
import { useEditStatus } from "../edit/plans";
import { AgentBadge, TypeBadge } from "../items/badges";
import { useErrorText } from "../items/text";
import { Compare } from "./Compare";
import { Contradictions } from "./Contradictions";
import { MergeDialog } from "./MergeDialog";

// Пороги сходства: «обычный» — значение по умолчанию в Go (similar.DefaultMinScore).
const THRESHOLDS = { low: 0.12, normal: 0.2, high: 0.35 } as const;
type Threshold = keyof typeof THRESHOLDS;

// Экран «Похожие»: группы кандидатов в дубли без LLM, сравнение рядом и
// объединение выбранных записей одной операцией.
export const SimilarPage = () => {
  const translate = useTranslate();
  const errorText = useErrorText();
  const [crossAgent, setCrossAgent] = useState(false);
  const [threshold, setThreshold] = useState<Threshold>("normal");
  const { data, error, isPending } = useQuery<SimilarGroup[]>({
    queryKey: ["similar", crossAgent, threshold],
    queryFn: async () =>
      (await call(() =>
        LibraryService.Similar({ projectId: "", crossAgent, minScore: THRESHOLDS[threshold] }),
      )) ?? [],
  });

  return (
    <Card sx={{ mt: 2 }}>
      <Title title={translate("aim.similar.title")} />
      <CardContent>
        <Stack spacing={2}>
          <div>
            <Typography variant="h5" component="h1">
              {translate("aim.similar.title")}
            </Typography>
            <Typography color="text.secondary">{translate("aim.similar.subtitle")}</Typography>
          </div>
          <Stack direction="row" spacing={2} sx={{ alignItems: "center", flexWrap: "wrap", rowGap: 1 }}>
            <FormControlLabel
              control={<Switch checked={crossAgent} onChange={(e) => setCrossAgent(e.target.checked)} />}
              label={translate("aim.similar.crossAgent")}
            />
            <TextField
              select
              size="small"
              label={translate("aim.similar.threshold")}
              value={threshold}
              onChange={(e) => setThreshold(e.target.value as Threshold)}
              sx={{ minWidth: 180 }}
            >
              {(Object.keys(THRESHOLDS) as Threshold[]).map((t) => (
                <MenuItem key={t} value={t}>
                  {translate(`aim.similar.thresholds.${t}`)}
                </MenuItem>
              ))}
            </TextField>
          </Stack>
          <Contradictions query={{ projectId: "", crossAgent, minScore: THRESHOLDS[threshold] }} />
          {error && <Alert severity="error">{errorText(error)}</Alert>}
          {isPending && <Typography>{translate("aim.similar.loading")}</Typography>}
          {data?.length === 0 && <Typography>{translate("aim.similar.empty")}</Typography>}
          {data?.map((g) => (
            <GroupCard key={(g.items ?? []).map((it) => it.id).join("|")} group={g} />
          ))}
        </Stack>
      </CardContent>
    </Card>
  );
};

const GroupCard = ({ group }: { group: SimilarGroup }) => {
  const translate = useTranslate();
  const { data: status } = useEditStatus();
  const { data: projects = [] } = useGetList<Project & { id: string }>("projects", {});
  const items = group.items ?? [];
  const [picked, setPicked] = useState<Set<string>>(() => new Set(items.map((it) => it.id)));
  const [compare, setCompare] = useState(false);
  const [merging, setMerging] = useState(false);
  const chosen = items.filter((it) => picked.has(it.id));
  const projectLabel = (id: string) => {
    const p = projects.find((x) => x.id === id);
    return p?.global ? translate("aim.projects.global") : p?.path || p?.label || id;
  };
  const toggle = (id: string) =>
    setPicked((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  return (
    <Paper variant="outlined" sx={{ p: 2 }}>
      <Stack spacing={1.5}>
        <Stack direction="row" spacing={1} sx={{ alignItems: "center" }}>
          <Chip
            size="small"
            color="primary"
            label={translate("aim.similar.score", { score: Math.round(group.score * 100) })}
          />
          <Box sx={{ flex: 1 }} />
          <Button size="small" onClick={() => setCompare((c) => !c)}>
            {translate(compare ? "aim.similar.hideCompare" : "aim.similar.compare")}
          </Button>
          <Button
            size="small"
            variant="contained"
            disabled={chosen.length < 2 || !status?.enabled}
            onClick={() => setMerging(true)}
          >
            {translate("aim.similar.merge")}
          </Button>
        </Stack>
        {items.map((it) => (
          <Stack key={it.id} direction="row" spacing={1} sx={{ alignItems: "center", minWidth: 0 }}>
            <Checkbox
              size="small"
              checked={picked.has(it.id)}
              onChange={() => toggle(it.id)}
              slotProps={{ input: { "aria-label": it.title } }}
            />
            <AgentBadge agent={it.agent} />
            <TypeBadge type={it.type} />
            <Typography variant="body2" sx={{ flex: 1, minWidth: 0, wordBreak: "break-word" }}>
              {it.title}
            </Typography>
            <Typography variant="caption" color="text.secondary" sx={{ wordBreak: "break-all" }}>
              {projectLabel(it.projectId)}
            </Typography>
          </Stack>
        ))}
        {compare && <Compare items={chosen.length > 0 ? chosen : items} />}
      </Stack>
      {merging && <MergeDialog items={chosen} onClose={() => setMerging(false)} />}
    </Paper>
  );
};
