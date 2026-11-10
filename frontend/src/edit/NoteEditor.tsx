import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslate } from "react-admin";
import { PlansService, type Draft } from "../api/bindings";
import { call } from "../api/errors";
import { useErrorText } from "../items/text";
import { CodeEditor } from "./CodeEditor";
import { useConsolidationHint, useEditStatus, useRunPlan } from "./plans";

// Запись в память Codex — только ad-hoc-заметками (ADR-002):
//  - create — новая заметка;
//  - refine — заметка, уточняющая запись Codex («Уточнить заметкой»);
//  - forget — заметка с просьбой исключить факт («Забыть»);
//  - edit   — правка нашей заметки целиком.
export type NoteTarget =
  | { kind: "create" }
  | { kind: "refine"; title: string }
  | { kind: "forget"; title: string }
  | { kind: "edit"; itemId: string };

export const NoteEditor = ({ target, onClose }: { target: NoteTarget; onClose: () => void }) => {
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
  return (
    <Dialog open onClose={onClose} maxWidth="md" fullWidth aria-labelledby="note-editor-title">
      <DialogTitle id="note-editor-title">{translate(`aim.note.titles.${target.kind}`)}</DialogTitle>
      {target.kind !== "edit" ? (
        <NewNoteForm target={target} onClose={onClose} />
      ) : draft.error ? (
        <DialogContent>
          <Alert severity="error">{errorText(draft.error)}</Alert>
        </DialogContent>
      ) : !draft.data ? (
        <DialogContent>
          <Typography color="text.secondary">{translate("aim.preview.loading")}</Typography>
        </DialogContent>
      ) : (
        <EditNoteForm itemId={target.itemId} draft={draft.data} onClose={onClose} />
      )}
    </Dialog>
  );
};

const ConsolidationAlert = () => {
  const translate = useTranslate();
  const hint = useConsolidationHint();
  const { data } = useEditStatus();
  return (
    <Alert severity="info">
      <Stack spacing={0.5}>
        <span>{translate("aim.codex.notesOnly", { generation: data?.codex.generation ?? "" })}</span>
        <span>{hint}</span>
      </Stack>
    </Alert>
  );
};

const NewNoteForm = ({
  target,
  onClose,
}: {
  target: Exclude<NoteTarget, { kind: "edit" }>;
  onClose: () => void;
}) => {
  const translate = useTranslate();
  const runPlan = useRunPlan();
  const [title, setTitle] = useState(() =>
    target.kind === "create"
      ? ""
      : translate(`aim.note.prefill.${target.kind}Title`, { title: target.title }),
  );
  const [body, setBody] = useState(() =>
    target.kind === "forget" ? translate("aim.note.prefill.forgetBody", { title: target.title }) : "",
  );
  const preview = () => {
    const build =
      target.kind === "forget"
        ? () => PlansService.ForgetNote(title, body)
        : () => PlansService.CreateNote(title, body);
    void runPlan(build, { onApplied: onClose });
  };
  return (
    <>
      <DialogContent dividers>
        <Stack spacing={2}>
          <ConsolidationAlert />
          {target.kind === "forget" && <Alert severity="warning">{translate("aim.note.forgetHelp")}</Alert>}
          <TextField
            label={translate("aim.note.fields.title")}
            value={title}
            onChange={(e) => setTitle(e.target.value)}
          />
          <CodeEditor label={translate("aim.note.fields.body")} value={body} onChange={setBody} />
          <Typography variant="body2" color="text.secondary">
            {translate("aim.note.bodyHelp")}
          </Typography>
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

const EditNoteForm = ({ itemId, draft, onClose }: { itemId: string; draft: Draft; onClose: () => void }) => {
  const translate = useTranslate();
  const runPlan = useRunPlan();
  const [baseHash] = useState(draft.hash);
  const [content, setContent] = useState(draft.content);
  return (
    <>
      <DialogContent dividers>
        <Stack spacing={2}>
          <ConsolidationAlert />
          <CodeEditor
            label={translate("aim.note.fields.content")}
            value={content}
            onChange={setContent}
            minHeight={360}
          />
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{translate("ra.action.cancel")}</Button>
        <Button
          variant="contained"
          onClick={() =>
            void runPlan(() => PlansService.UpdateSource(itemId, baseHash, content), { onApplied: onClose })
          }
        >
          {translate("aim.plan.preview")}
        </Button>
      </DialogActions>
    </>
  );
};
