import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Stack,
  Typography,
} from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslate } from "react-admin";
import { PlansService, type Draft, type Item } from "../api/bindings";
import { call } from "../api/errors";
import { useErrorText } from "../items/text";
import { CodeEditor } from "./CodeEditor";
import { useRunPlan } from "./plans";

// FileEditor — правка файла инструкций или SKILL.md навыка целиком (ADR-020).
// Пишет тем же путём, что память: план с diff, снимок, откат в истории.
// О чужом git-репозитории и общем навыке предупреждает окно плана.
export const FileEditor = ({ item, onClose }: { item: Item; onClose: () => void }) => {
  const translate = useTranslate();
  const errorText = useErrorText();
  const draft = useQuery<Draft>({
    queryKey: ["plans", "draft", item.id],
    queryFn: () => call(() => PlansService.Draft(item.id)),
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
  return (
    <Dialog open onClose={onClose} maxWidth="md" fullWidth aria-labelledby="file-editor-title">
      <DialogTitle id="file-editor-title">
        {translate(`aim.file.titles.${item.kind}`, { title: item.title })}
      </DialogTitle>
      {draft.error ? (
        <DialogContent>
          <Alert severity="error">{errorText(draft.error)}</Alert>
        </DialogContent>
      ) : !draft.data ? (
        <DialogContent>
          <Typography color="text.secondary">{translate("aim.preview.loading")}</Typography>
        </DialogContent>
      ) : (
        <FileForm item={item} draft={draft.data} onClose={onClose} />
      )}
    </Dialog>
  );
};

const FileForm = ({ item, draft, onClose }: { item: Item; draft: Draft; onClose: () => void }) => {
  const translate = useTranslate();
  const runPlan = useRunPlan();
  const [content, setContent] = useState(draft.content);
  return (
    <>
      <DialogContent dividers>
        <Stack spacing={2}>
          <Typography
            variant="body2"
            color="text.secondary"
            sx={{ fontFamily: "monospace", wordBreak: "break-all" }}
          >
            {item.file}
          </Typography>
          <CodeEditor label={translate("aim.file.content")} value={content} onChange={setContent} />
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{translate("ra.action.cancel")}</Button>
        <Button
          variant="contained"
          disabled={content === draft.content}
          onClick={() =>
            void runPlan(() => PlansService.UpdateSource(item.id, draft.hash, content), {
              onApplied: onClose,
            })
          }
        >
          {translate("aim.plan.preview")}
        </Button>
      </DialogActions>
    </>
  );
};
