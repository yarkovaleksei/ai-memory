import { Alert, Box, Chip, Divider, Stack, Tab, Tabs, Tooltip, Typography } from "@mui/material";
import { useState } from "react";
import { DateField, RecordContextProvider, useGetOne, useTranslate } from "react-admin";
import type { Item, Project, Source } from "../api/bindings";
import { ItemActions } from "../edit/ItemActions";
import { useConsolidationHint } from "../edit/plans";
import { AgentBadge, TypeBadge } from "./badges";
import { Markdown } from "./Markdown";
import { useErrorText, useWarningText } from "./text";

type Rec<T> = T & { id: string };

const FULL_ITEM = { full: true };

interface ItemPreviewProps {
  id: string | null;
  onOpenItem: (id: string) => void;
}

// Просмотр записи справа от списка: текст с рабочими ссылками на другие записи,
// исходник с метаданными и действия правки, доступные для этого вида записи.
export const ItemPreview = ({ id, onOpenItem }: ItemPreviewProps) => {
  const translate = useTranslate();
  const errorText = useErrorText();
  // react-admin кладёт записи списка в кэш getOne, а в списке тела нет. meta даёт
  // отдельный ключ кэша, и просмотр всегда запрашивает запись целиком.
  const { data, isPending, error } = useGetOne<Rec<Item>>(
    "items",
    { id: id ?? "", meta: FULL_ITEM },
    { enabled: !!id },
  );

  if (!id) {
    return <Placeholder text={translate("aim.preview.empty")} />;
  }
  if (error) {
    return (
      <Alert severity="warning" sx={{ m: 2 }}>
        {errorText(error)}
      </Alert>
    );
  }
  if (isPending || !data) {
    return <Placeholder text={translate("aim.preview.loading")} />;
  }
  // key сбрасывает выбранную вкладку при переходе к другой записи.
  return <ItemView key={data.id} item={data} onOpenItem={onOpenItem} />;
};

// Заметка, которую Codex ещё не учёл: подсказка говорит, когда учтёт.
export const PendingChip = () => {
  const translate = useTranslate();
  const hint = useConsolidationHint();
  return (
    <Tooltip title={hint}>
      <Chip size="small" color="warning" variant="outlined" label={translate("aim.codex.pending")} />
    </Tooltip>
  );
};

const Placeholder = ({ text }: { text: string }) => (
  <Box sx={{ p: 3 }}>
    <Typography color="text.secondary">{text}</Typography>
  </Box>
);

const ItemView = ({ item, onOpenItem }: { item: Rec<Item>; onOpenItem: (id: string) => void }) => {
  const translate = useTranslate();
  const warningText = useWarningText();
  const [tab, setTab] = useState<"text" | "source">("text");
  const { data: source } = useGetOne<Rec<Source>>(
    "sources",
    { id: item.sourceId },
    { enabled: !!item.sourceId },
  );
  const { data: project } = useGetOne<Rec<Project>>(
    "projects",
    { id: item.projectId },
    { enabled: !!item.projectId },
  );
  const warnings = item.warnings ?? [];
  const projectLabel = project?.global ? translate("aim.projects.global") : project?.path || project?.label;

  return (
    <RecordContextProvider value={item}>
      <Stack spacing={1.5} sx={{ p: 2, minWidth: 0 }}>
        <Typography variant="h6" component="h2" sx={{ wordBreak: "break-word" }}>
          {item.title}
        </Typography>
        <Stack direction="row" spacing={1} sx={{ flexWrap: "wrap", alignItems: "center", rowGap: 1 }}>
          <AgentBadge agent={item.agent} />
          <TypeBadge type={item.type} />
          <Chip
            size="small"
            variant="outlined"
            label={translate(`aim.kinds.${item.kind}`, { _: item.kind })}
          />
          <Chip
            size="small"
            variant="outlined"
            label={translate(`aim.write.${item.write}`, { _: item.write })}
          />
          {item.pending && <PendingChip />}
          <Typography variant="body2" color="text.secondary" component="span">
            <DateField source="modTime" showTime />
          </Typography>
        </Stack>
        <ItemActions item={item} />
        {item.description && <Typography color="text.secondary">{item.description}</Typography>}
        {warnings.length > 0 && (
          <Alert severity="warning">
            <Stack spacing={0.5}>
              {warnings.map((w, i) => (
                <span key={`${w.code}-${i}`}>{warningText(w)}</span>
              ))}
            </Stack>
          </Alert>
        )}
        <Tabs value={tab} onChange={(_, v: "text" | "source") => setTab(v)}>
          <Tab value="text" label={translate("aim.preview.tabs.text")} />
          <Tab value="source" label={translate("aim.preview.tabs.source")} />
        </Tabs>
        {tab === "text" ? (
          item.body.trim() ? (
            <Markdown body={item.body} links={item.links ?? []} onOpenItem={onOpenItem} />
          ) : (
            <Typography color="text.secondary">{translate("aim.preview.noBody")}</Typography>
          )
        ) : (
          <SourceView item={item} />
        )}
        <Divider />
        <Meta label={translate("aim.preview.project")} value={projectLabel} />
        <Meta
          label={translate("aim.preview.source")}
          value={source ? `${source.label} — ${source.root}` : ""}
        />
        <Meta label={translate("aim.preview.file")} value={item.file} mono />
        <Meta label={translate("aim.preview.anchor")} value={item.anchor} />
      </Stack>
    </RecordContextProvider>
  );
};

const SourceView = ({ item }: { item: Item }) => {
  const translate = useTranslate();
  const fm = Object.entries(item.frontmatter ?? {});
  return (
    <Stack spacing={1}>
      {fm.length > 0 && (
        <>
          <Typography variant="subtitle2">{translate("aim.preview.frontmatter")}</Typography>
          <Box component="pre" sx={preSx}>
            {fm.map(([k, v]) => `${k}: ${typeof v === "string" ? v : JSON.stringify(v)}`).join("\n")}
          </Box>
          <Typography variant="subtitle2">{translate("aim.preview.body")}</Typography>
        </>
      )}
      <Box component="pre" sx={preSx}>
        {item.body}
      </Box>
    </Stack>
  );
};

const preSx = {
  m: 0,
  p: 1,
  bgcolor: "action.hover",
  borderRadius: 1,
  fontFamily: "monospace",
  fontSize: "0.85rem",
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
} as const;

const Meta = ({ label, value, mono }: { label: string; value?: string; mono?: boolean }) =>
  value ? (
    <Box>
      <Typography variant="caption" color="text.secondary">
        {label}
      </Typography>
      <Typography variant="body2" sx={{ wordBreak: "break-all", fontFamily: mono ? "monospace" : undefined }}>
        {value}
      </Typography>
    </Box>
  ) : null;
