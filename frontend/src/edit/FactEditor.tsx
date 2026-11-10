import {
  Alert,
  Autocomplete,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  MenuItem,
  Stack,
  Tab,
  Tabs,
  TextField,
  Typography,
} from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
import { useGetList, useTranslate } from "react-admin";
import {
  PlansService,
  type Draft,
  type FactInput,
  type Plan,
  type Project,
  type Source,
} from "../api/bindings";
import { call } from "../api/errors";
import { useErrorText } from "../items/text";
import { CodeEditor } from "./CodeEditor";
import { useRunPlan } from "./plans";

export type FactTarget = { kind: "create"; projectId?: string } | { kind: "edit"; itemId: string };

const TYPES = ["user", "feedback", "project", "reference"] as const;

const emptyFact: FactInput = { name: "", description: "", type: "project", title: "", hook: "", body: "" };

// Форма факта Claude: поля frontmatter и строки MEMORY.md плюс текст в
// CodeMirror. «Показать изменения» строит план в Go и открывает diff — до
// подтверждения ничего не пишется.
export const FactEditor = ({ target, onClose }: { target: FactTarget; onClose: () => void }) => {
  const translate = useTranslate();
  const errorText = useErrorText();
  const itemId = target.kind === "edit" ? target.itemId : "";
  const draft = useQuery<Draft>({
    queryKey: ["plans", "draft", itemId],
    queryFn: () => call(() => PlansService.Draft(itemId)),
    enabled: target.kind === "edit",
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
  const title = translate(target.kind === "create" ? "aim.fact.createTitle" : "aim.fact.editTitle");

  return (
    <Dialog open onClose={onClose} maxWidth="md" fullWidth aria-labelledby="fact-editor-title">
      <DialogTitle id="fact-editor-title">{title}</DialogTitle>
      {target.kind === "create" ? (
        <FactForm projectId={target.projectId} onClose={onClose} />
      ) : draft.error ? (
        <DialogContent>
          <Alert severity="error">{errorText(draft.error)}</Alert>
        </DialogContent>
      ) : !draft.data ? (
        <DialogContent>
          <Typography color="text.secondary">{translate("aim.preview.loading")}</Typography>
        </DialogContent>
      ) : (
        <FactForm itemId={target.itemId} draft={draft.data} onClose={onClose} />
      )}
    </Dialog>
  );
};

interface FactFormProps {
  itemId?: string;
  draft?: Draft;
  projectId?: string;
  onClose: () => void;
  onApplied?: () => void; // после записи; по умолчанию — onClose
  // Для переноса: поля заполнены из исходной записи, план строит перенос.
  initial?: Partial<FactInput>;
  build?: (sourceId: string, projectId: string, fact: FactInput) => Promise<Plan>;
  extra?: ReactNode; // над полями: выбор режима переноса и предупреждения
  // Поля, вписываемые в открытую форму, например черновик от LLM. Новый seq —
  // новая вставка; остальное, что пользователь уже выбрал, остаётся.
  patch?: { seq: number; fact: Partial<FactInput> };
}

export const FactForm = ({
  itemId,
  draft,
  projectId,
  onClose,
  onApplied,
  initial,
  build: buildNew,
  extra,
  patch,
}: FactFormProps) => {
  const translate = useTranslate();
  const runPlan = useRunPlan();
  const editing = !!itemId && !!draft;
  // Хэш фиксируется при открытии: если файл изменят, пока форма открыта, план
  // не построится и не затрёт чужую правку — даже когда черновик перечитан.
  const [baseHash] = useState(draft?.hash ?? "");
  const [fact, setFact] = useState<FactInput>(() => ({ ...emptyFact, ...initial, ...(draft?.fact ?? {}) }));
  const [content, setContent] = useState(draft?.content ?? "");
  const [tab, setTab] = useState<"form" | "source">(draft && !draft.fact ? "source" : "form");
  const [project, setProject] = useState(projectId ?? "");
  const [source, setSource] = useState("");
  const [patchSeq, setPatchSeq] = useState(patch?.seq ?? 0);
  if (patch && patch.seq !== patchSeq) {
    setPatchSeq(patch.seq);
    setFact((f) => ({ ...f, ...patch.fact }));
  }

  const { data: projects = [] } = useGetList<Project & { id: string }>("projects", {}, { enabled: !editing });
  const { data: sources = [] } = useGetList<Source & { id: string }>("sources", {}, { enabled: !editing });
  const claudeSources = sources.filter((s) => s.agent === "claude" && !s.readOnly);
  const sourceId = source || claudeSources[0]?.id || "";
  const projectChoices = projects.filter((p) => !p.global);

  const set = (k: keyof FactInput) => (e: { target: { value: string } }) =>
    setFact({ ...fact, [k]: e.target.value });

  const preview = () => {
    let build;
    if (!editing && buildNew) build = () => buildNew(sourceId, project, fact);
    else if (!editing) build = () => PlansService.CreateFact(sourceId, project, fact);
    else if (tab === "source") build = () => PlansService.UpdateSource(itemId, baseHash, content);
    else build = () => PlansService.UpdateFact(itemId, baseHash, fact);
    void runPlan(build, { onApplied: onApplied ?? onClose });
  };

  return (
    <>
      <DialogContent dividers>
        <Stack spacing={2}>
          {extra}
          {editing && (
            <Tabs value={tab} onChange={(_, v: "form" | "source") => setTab(v)}>
              <Tab value="form" label={translate("aim.fact.tabs.form")} disabled={!draft.fact} />
              <Tab value="source" label={translate("aim.fact.tabs.source")} />
            </Tabs>
          )}
          {editing && !draft.fact && (
            <Alert severity="warning">{translate("aim.fact.brokenFrontmatter")}</Alert>
          )}
          {!editing && (
            <Stack direction="row" spacing={2}>
              {/* Проектов десятки: поле с поиском, а не список на весь экран. */}
              <Autocomplete
                sx={{ flex: 2 }}
                options={projectChoices}
                value={projectChoices.find((p) => p.id === project) ?? null}
                onChange={(_, p) => setProject(p?.id ?? "")}
                getOptionLabel={(p) => p.path || p.label}
                isOptionEqualToValue={(a, b) => a.id === b.id}
                noOptionsText={translate("aim.projects.notFound")}
                renderInput={(params) => (
                  <TextField
                    {...params}
                    label={translate("aim.fact.fields.project")}
                    helperText={translate("aim.fact.help.project")}
                  />
                )}
              />
              <TextField
                select
                label={translate("aim.fact.fields.profile")}
                value={sourceId}
                onChange={(e) => setSource(e.target.value)}
                sx={{ flex: 1 }}
              >
                {claudeSources.map((s) => (
                  <MenuItem key={s.id} value={s.id}>
                    {s.label}
                  </MenuItem>
                ))}
              </TextField>
            </Stack>
          )}
          {tab === "form" ? (
            <>
              <Stack direction="row" spacing={2}>
                <TextField
                  label={translate("aim.fact.fields.name")}
                  value={fact.name}
                  onChange={set("name")}
                  disabled={editing}
                  helperText={translate("aim.fact.help.name")}
                  sx={{ flex: 1 }}
                />
                <TextField
                  select
                  label={translate("aim.fact.fields.type")}
                  value={fact.type}
                  onChange={set("type")}
                  sx={{ width: 200 }}
                >
                  {TYPES.map((t) => (
                    <MenuItem key={t} value={t}>
                      {translate(`aim.types.${t}`)}
                    </MenuItem>
                  ))}
                </TextField>
              </Stack>
              <TextField
                label={translate("aim.fact.fields.description")}
                value={fact.description}
                onChange={set("description")}
                helperText={translate("aim.fact.help.description")}
              />
              <Stack direction="row" spacing={2}>
                <TextField
                  label={translate("aim.fact.fields.title")}
                  value={fact.title}
                  onChange={set("title")}
                  sx={{ flex: 1 }}
                />
                <TextField
                  label={translate("aim.fact.fields.hook")}
                  value={fact.hook}
                  onChange={set("hook")}
                  helperText={translate("aim.fact.help.hook")}
                  sx={{ flex: 1 }}
                />
              </Stack>
              <CodeEditor
                label={translate("aim.fact.fields.body")}
                value={fact.body}
                onChange={(body) => setFact((f) => ({ ...f, body }))}
              />
            </>
          ) : (
            <CodeEditor
              label={translate("aim.fact.fields.raw")}
              value={content}
              onChange={setContent}
              minHeight={360}
            />
          )}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{translate("ra.action.cancel")}</Button>
        <Button variant="contained" onClick={preview}>
          {translate("aim.plan.preview")}
        </Button>
      </DialogActions>
    </>
  );
};
