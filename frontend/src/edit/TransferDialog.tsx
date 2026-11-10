import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Stack,
  Switch,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
} from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
import { useGetOne, useTranslate } from "react-admin";
import { HistoryService, PlansService, type Item, type Project } from "../api/bindings";
import { call } from "../api/errors";
import { CodeEditor } from "./CodeEditor";
import { FactForm } from "./FactEditor";
import { ConsolidationAlert } from "./NoteEditor";
import { useRunPlan } from "./plans";

type Target = "claude" | "codex";

const TRANSLIT: Record<string, string> = {
  а: "a",
  б: "b",
  в: "v",
  г: "g",
  д: "d",
  е: "e",
  ё: "e",
  ж: "zh",
  з: "z",
  и: "i",
  й: "y",
  к: "k",
  л: "l",
  м: "m",
  н: "n",
  о: "o",
  п: "p",
  р: "r",
  с: "s",
  т: "t",
  у: "u",
  ф: "f",
  х: "h",
  ц: "ts",
  ч: "ch",
  ш: "sh",
  щ: "sch",
  ъ: "",
  ы: "y",
  ь: "",
  э: "e",
  ю: "yu",
  я: "ya",
};

// factName предлагает имя файла факта из заголовка: латиница, цифры и «-», как
// требует Go (claude.fact.name-invalid). Пользователь может его поправить.
export const factName = (title: string) =>
  [...title.toLowerCase()]
    .map((ch) => (/[a-z0-9]/.test(ch) ? ch : (TRANSLIT[ch] ?? "-")))
    .join("")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "")
    .slice(0, 60) || "fact";

const isFact = (it: Item) => it.kind === "claude-fact" && it.write === "direct";

// TransferDialog — перенос или копия записи в другой агент, проект или профиль.
// Копия оставляет источник, перенос убирает его: факт Claude удаляется, для
// записи Codex пишется заметка «Забыть». Всё одним планом с diff (ADR-014).
export const TransferDialog = ({ item, onClose }: { item: Item; onClose: () => void }) => {
  const translate = useTranslate();
  const targets: Target[] = item.agent === "claude" ? ["codex", "claude"] : ["claude"];
  const [target, setTarget] = useState<Target>(targets[0] ?? "claude");
  const canMove = isFact(item) || (item.agent === "codex" && item.write !== "none");
  const [move, setMove] = useState(false);
  const { data: links = [] } = useQuery({
    queryKey: ["history", "links", item.id],
    queryFn: async () => (await call(() => HistoryService.Links(item.id))) ?? [],
  });
  const already = links.filter((l) => l.direction === "to" && l.agent === target);

  const extra = (
    <Stack spacing={1.5}>
      <Stack direction="row" spacing={2} sx={{ alignItems: "center", flexWrap: "wrap", rowGap: 1 }}>
        <ToggleButtonGroup
          size="small"
          exclusive
          value={target}
          onChange={(_, v: Target | null) => v && setTarget(v)}
          aria-label={translate("aim.transfer.target")}
        >
          {targets.map((t) => (
            <ToggleButton key={t} value={t}>
              {translate(`aim.transfer.targets.${t}`)}
            </ToggleButton>
          ))}
        </ToggleButtonGroup>
        <FormControlLabel
          control={<Switch checked={move} disabled={!canMove} onChange={(e) => setMove(e.target.checked)} />}
          label={translate("aim.transfer.move")}
        />
      </Stack>
      <Alert severity={move ? "warning" : "info"}>
        {translate(move ? `aim.transfer.moveHelp.${item.agent}` : "aim.transfer.copyHelp")}
      </Alert>
      {already.map((l) => (
        <Alert key={l.commit} severity="warning">
          {translate("aim.transfer.already", { title: l.title, time: new Date(l.time).toLocaleString() })}
        </Alert>
      ))}
    </Stack>
  );

  return (
    <Dialog open onClose={onClose} maxWidth="md" fullWidth aria-labelledby="transfer-title">
      <DialogTitle id="transfer-title">{translate("aim.transfer.title", { title: item.title })}</DialogTitle>
      {target === "claude" ? (
        <ToClaude key="claude" item={item} move={move} extra={extra} onClose={onClose} />
      ) : (
        <ToCodex key="codex" item={item} move={move} extra={extra} onClose={onClose} />
      )}
    </Dialog>
  );
};

interface ToProps {
  item: Item;
  move: boolean;
  extra: ReactNode;
  onClose: () => void;
}

const ToClaude = ({ item, move, extra, onClose }: ToProps) => {
  const translate = useTranslate();
  const forgetTitle = translate("aim.note.prefill.forgetTitle", { title: item.title });
  const forgetBody = translate("aim.transfer.forgetBody", { title: item.title });
  const description = item.description || item.title;
  return (
    <FactForm
      // Глобальная запись Codex проекта не имеет: проект выбирает пользователь.
      projectId={item.projectId === "global" ? "" : item.projectId}
      initial={{
        name: factName(item.title),
        title: item.title,
        description: description.split("\n")[0] ?? "",
        type: item.type || "project",
        body: item.body,
      }}
      build={(sourceId, projectId, fact) =>
        PlansService.TransferToClaude(item.id, sourceId, projectId, fact, move, forgetTitle, forgetBody)
      }
      extra={extra}
      onClose={onClose}
    />
  );
};

const ToCodex = ({ item, move, extra, onClose }: ToProps) => {
  const translate = useTranslate();
  const runPlan = useRunPlan();
  const { data: project } = useGetOne<Project & { id: string }>(
    "projects",
    { id: item.projectId },
    { enabled: !!item.projectId },
  );
  // Заметки Codex глобальные: проект и источник пишутся в текст, иначе факт
  // потеряет привязку.
  const origin = translate("aim.transfer.origin", {
    title: item.title,
    project: project?.path || project?.label || translate("aim.projects.global"),
  });
  const [title, setTitle] = useState(item.title);
  const [body, setBody] = useState(`${item.body.trim()}\n\n${origin}`);
  return (
    <>
      <DialogContent dividers>
        <Stack spacing={2}>
          {extra}
          <ConsolidationAlert />
          <TextField
            label={translate("aim.note.fields.title")}
            value={title}
            onChange={(e) => setTitle(e.target.value)}
          />
          <CodeEditor label={translate("aim.note.fields.body")} value={body} onChange={setBody} />
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{translate("ra.action.cancel")}</Button>
        <Button
          variant="contained"
          onClick={() =>
            void runPlan(() => PlansService.TransferToCodex(item.id, title, body, move), {
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
