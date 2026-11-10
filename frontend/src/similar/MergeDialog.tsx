import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Stack,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from "@mui/material";
import { useState, type ReactNode } from "react";
import { useNotify, useTranslate } from "react-admin";
import {
  Agent,
  LLMService,
  PlansService,
  type FactInput,
  type Item,
  type MergeDraft,
  type NoteText,
} from "../api/bindings";
import { CodeEditor } from "../edit/CodeEditor";
import { FactForm } from "../edit/FactEditor";
import { ConsolidationAlert } from "../edit/NoteEditor";
import { useRunPlan } from "../edit/plans";
import { factName } from "../edit/TransferDialog";
import { LLMActions, useLLMCall } from "../llm/llm";

type Target = "claude" | "codex";

// Черновик без LLM — тексты записей подряд: пользователь сводит их сам или
// просит черновик у модели. Ничего не теряется, пока он не нажмёт «Записать».
const joinBodies = (items: Item[]) =>
  items
    .map((it) => it.body.trim())
    .filter(Boolean)
    .join("\n\n");

// Пустой заголовок или описание из ответа не затирают то, что уже в форме.
const draftFact = (d: MergeDraft): Partial<FactInput> => ({
  ...(d.title ? { title: d.title } : {}),
  ...(d.description ? { description: d.description } : {}),
  body: d.body,
});

// Черновик от модели с номером вставки: форма вписывает его поверх полей.
interface Drafted {
  seq: number;
  draft: MergeDraft;
}

// MergeDialog — объединение группы: одна новая запись, факты Claude группы
// удаляются, для записей Codex — заметки «Забыть». Всё одним планом.
export const MergeDialog = ({ items, onClose }: { items: Item[]; onClose: () => void }) => {
  const translate = useTranslate();
  const notify = useNotify();
  const facts = items.filter((it) => it.agent === "claude");
  const [target, setTarget] = useState<Target>(facts.length > 0 ? "claude" : "codex");
  const [drafted, setDrafted] = useState<Drafted | null>(null);
  const llm = useLLMCall<MergeDraft>();
  const forget: NoteText[] = items
    .filter((it) => it.agent === "codex")
    .map((it) => ({
      itemId: it.id,
      title: translate("aim.note.prefill.forgetTitle", { title: it.title }),
      body: translate("aim.merge.forgetBody", { title: it.title }),
    }));
  const ids = items.map((it) => it.id);

  const askDraft = async (agent: Agent) => {
    const draft = await llm.run(agent, (a) => LLMService.MergeDraft(ids, a));
    if (!draft) return;
    setDrafted((d) => ({ seq: (d?.seq ?? 0) + 1, draft }));
    notify("aim.llm.drafted", { type: "info", messageArgs: { agent: translate(`aim.llm.agents.${agent}`) } });
  };

  const extra = (
    <Stack spacing={1.5}>
      <ToggleButtonGroup
        size="small"
        exclusive
        value={target}
        onChange={(_, v: Target | null) => v && setTarget(v)}
        aria-label={translate("aim.transfer.target")}
      >
        <ToggleButton value="claude">{translate("aim.merge.targets.claude")}</ToggleButton>
        <ToggleButton value="codex">{translate("aim.merge.targets.codex")}</ToggleButton>
      </ToggleButtonGroup>
      <Alert severity="warning">
        {translate("aim.merge.effect", {
          facts: target === "claude" ? Math.max(facts.length - 1, 0) : facts.length,
          notes: forget.length,
        })}
      </Alert>
      <Typography variant="body2" color="text.secondary">
        {translate("aim.llm.draftHelp")}
      </Typography>
      <LLMActions
        label="aim.llm.draft"
        busy="aim.llm.drafting"
        running={llm.running}
        error={llm.error}
        onRun={(a) => void askDraft(a)}
        onStop={llm.stop}
      />
      {drafted && <Disagreements draft={drafted.draft} items={items} />}
    </Stack>
  );

  const first = facts[0] ?? items[0];
  const d = drafted?.draft;
  return (
    <Dialog open onClose={onClose} maxWidth="md" fullWidth aria-labelledby="merge-title">
      <DialogTitle id="merge-title">
        {translate("aim.merge.title", { smart_count: items.length })}
      </DialogTitle>
      {target === "claude" ? (
        <FactForm
          key="claude"
          projectId={first && first.projectId !== "global" ? first.projectId : ""}
          initial={{
            name: (facts[0]?.frontmatter?.name as string | undefined) ?? factName(first?.title ?? ""),
            title: d?.title || first?.title || "",
            description: d?.description || first?.description || "",
            type: first?.type || "project",
            body: d?.body ?? joinBodies(items),
          }}
          patch={drafted ? { seq: drafted.seq, fact: draftFact(drafted.draft) } : undefined}
          build={(sourceId, projectId, fact) =>
            PlansService.Merge({
              itemIds: ids,
              target: Agent.AgentClaude,
              sourceId,
              projectId,
              fact,
              title: "",
              body: "",
              forget,
            })
          }
          extra={extra}
          onClose={onClose}
        />
      ) : (
        <ToCodex
          key="codex"
          items={items}
          drafted={drafted}
          extra={extra}
          forget={forget}
          onClose={onClose}
        />
      )}
    </Dialog>
  );
};

// Disagreements — противоречия, которые модель нашла между записями группы:
// она оставила оба варианта в черновике, выбирает человек.
const Disagreements = ({ draft, items }: { draft: MergeDraft; items: Item[] }) => {
  const translate = useTranslate();
  const list = draft.contradictions ?? [];
  if (list.length === 0) return null;
  const title = (id: string) => items.find((it) => it.id === id)?.title ?? id;
  return (
    <Alert severity="warning">
      {translate("aim.llm.contradictions")}
      <ul style={{ margin: 0, paddingLeft: 20 }}>
        {list.map((c, i) => (
          <li key={i}>
            {translate("aim.llm.between", {
              summary: c.summary,
              titles: (c.itemIds ?? []).map((id) => `«${title(id)}»`).join(", "),
            })}
          </li>
        ))}
      </ul>
    </Alert>
  );
};

const emptyFact = { name: "", description: "", type: "", title: "", hook: "", body: "" };

const ToCodex = ({
  items,
  drafted,
  extra,
  forget,
  onClose,
}: {
  items: Item[];
  drafted: Drafted | null;
  extra: ReactNode;
  forget: NoteText[];
  onClose: () => void;
}) => {
  const translate = useTranslate();
  const runPlan = useRunPlan();
  const [title, setTitle] = useState(drafted?.draft.title || items[0]?.title || "");
  const [body, setBody] = useState(drafted?.draft.body ?? joinBodies(items));
  const [seq, setSeq] = useState(drafted?.seq ?? 0);
  if (drafted && drafted.seq !== seq) {
    setSeq(drafted.seq);
    setTitle(drafted.draft.title || title);
    setBody(drafted.draft.body);
  }
  const merge = () =>
    PlansService.Merge({
      itemIds: items.map((it) => it.id),
      target: Agent.AgentCodex,
      sourceId: "",
      projectId: "",
      fact: emptyFact,
      title,
      body,
      forget,
    });
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
        <Button variant="contained" onClick={() => void runPlan(merge, { onApplied: onClose })}>
          {translate("aim.plan.preview")}
        </Button>
      </DialogActions>
    </>
  );
};
