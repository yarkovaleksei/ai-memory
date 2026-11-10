import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  MenuItem,
  Stack,
  Switch,
  TextField,
  Typography,
} from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslate } from "react-admin";
import {
  PlansService,
  type Item,
  type Section,
  type SectionTarget,
  type SectionsDraft,
} from "../api/bindings";
import { call } from "../api/errors";
import { useErrorText } from "../items/text";
import { useRunPlan } from "./plans";

// SectionDialog — перенос правила между файлами инструкций (ADR-021): раздел
// по заголовку вставляется в другой файл того же проекта, обычно из CLAUDE.md
// в AGENTS.md и обратно. Предпросмотр — окно плана с diff; там же
// предупреждения об импортах «@», которых Codex не понимает. Go получает
// только ID записи, ключ цели, номера разделов и хэши.
export const SectionDialog = ({ item, onClose }: { item: Item; onClose: () => void }) => {
  const translate = useTranslate();
  const errorText = useErrorText();
  const sections = useQuery<SectionsDraft>({
    queryKey: ["plans", "sections", item.id],
    queryFn: () => call(() => PlansService.InstructionSections(item.id)),
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
  const targets = useQuery<SectionTarget[]>({
    queryKey: ["plans", "sectionTargets", item.id],
    queryFn: async () => (await call(() => PlansService.SectionTargets(item.id))) ?? [],
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
  const error = sections.error ?? targets.error;
  return (
    <Dialog open onClose={onClose} maxWidth="md" fullWidth aria-labelledby="section-title">
      <DialogTitle id="section-title">{translate("aim.section.title", { title: item.title })}</DialogTitle>
      {error ? (
        <DialogContent>
          <Alert severity="error">{errorText(error)}</Alert>
        </DialogContent>
      ) : !sections.data || !targets.data ? (
        <DialogContent>
          <Typography color="text.secondary">{translate("aim.preview.loading")}</Typography>
        </DialogContent>
      ) : (
        <SectionForm item={item} draft={sections.data} targets={targets.data} onClose={onClose} />
      )}
    </Dialog>
  );
};

const indent = (level: number) => "  ".repeat(Math.max(level - 1, 0));

const useSectionLabel = () => {
  const translate = useTranslate();
  return (s: Section) => indent(s.level) + (s.heading || translate("aim.section.preamble"));
};

interface FormProps {
  item: Item;
  draft: SectionsDraft;
  targets: SectionTarget[];
  onClose: () => void;
}

const SectionForm = ({ item, draft, targets, onClose }: FormProps) => {
  const translate = useTranslate();
  const runPlan = useRunPlan();
  const label = useSectionLabel();
  const list = draft.sections ?? [];
  // По умолчанию — первый раздел второго уровня: верхний «# Заголовок»
  // обычно накрывает весь файл, а правило — это его подраздел.
  const [index, setIndex] = useState(() => (list.find((s) => s.level > 1) ?? list[0])?.index ?? -1);
  const [key, setKey] = useState(targets[0]?.key ?? "");
  const [after, setAfter] = useState(-1);
  const [move, setMove] = useState(false);

  if (list.length === 0) {
    return <Empty onClose={onClose} text={translate("aim.section.noSections")} />;
  }
  if (targets.length === 0) {
    return <Empty onClose={onClose} text={translate("aim.section.noTargets")} />;
  }
  const section = list.find((s) => s.index === index);
  const target = targets.find((t) => t.key === key);
  const imports = section?.imports ?? [];

  return (
    <>
      <DialogContent dividers>
        <Stack spacing={2}>
          <TextField
            select
            label={translate("aim.section.section")}
            value={index}
            onChange={(e) => setIndex(Number(e.target.value))}
          >
            {list.map((s) => (
              <MenuItem key={s.index} value={s.index}>
                {label(s)}
              </MenuItem>
            ))}
          </TextField>
          {section && (
            <Box
              component="pre"
              aria-label={translate("aim.section.text")}
              sx={{
                m: 0,
                p: 1.5,
                maxHeight: 240,
                overflow: "auto",
                bgcolor: "action.hover",
                borderRadius: 1,
                fontFamily: "monospace",
                fontSize: 13,
                whiteSpace: "pre-wrap",
                wordBreak: "break-word",
              }}
            >
              {section.text}
            </Box>
          )}
          {imports.length > 0 && target?.agent === "codex" && (
            <Alert severity="warning">
              {translate("aim.section.imports", { imports: imports.join(", ") })}
            </Alert>
          )}
          <TextField
            select
            label={translate("aim.section.target")}
            value={key}
            onChange={(e) => {
              setKey(e.target.value);
              setAfter(-1);
            }}
            helperText={target?.file}
          >
            {targets.map((t) => (
              <MenuItem key={t.key} value={t.key}>
                {translate(t.exists ? "aim.section.targetLabel" : "aim.section.targetNew", {
                  agent: translate(`aim.agents.${t.agent}`),
                  title: t.title,
                })}
              </MenuItem>
            ))}
          </TextField>
          <TextField
            select
            label={translate("aim.section.position")}
            value={after}
            onChange={(e) => setAfter(Number(e.target.value))}
            disabled={!target?.exists}
          >
            <MenuItem value={-1}>{translate("aim.section.end")}</MenuItem>
            {(target?.sections ?? []).map((s) => (
              <MenuItem key={s.index} value={s.index}>
                {translate("aim.section.after", { heading: label(s) })}
              </MenuItem>
            ))}
          </TextField>
          <FormControlLabel
            control={
              <Switch
                checked={move}
                disabled={item.write !== "direct"}
                onChange={(e) => setMove(e.target.checked)}
              />
            }
            label={translate("aim.section.move")}
          />
          <Alert severity={move ? "warning" : "info"}>
            {translate(move ? "aim.section.moveHelp" : "aim.section.copyHelp", { title: item.title })}
          </Alert>
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{translate("ra.action.cancel")}</Button>
        <Button
          variant="contained"
          disabled={!section || !target}
          onClick={() =>
            target &&
            void runPlan(
              () =>
                PlansService.CopySection(item.id, draft.hash, index, target.key, target.hash, after, move),
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

const Empty = ({ text, onClose }: { text: string; onClose: () => void }) => {
  const translate = useTranslate();
  return (
    <>
      <DialogContent>
        <Alert severity="info">{text}</Alert>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{translate("ra.action.close")}</Button>
      </DialogActions>
    </>
  );
};
