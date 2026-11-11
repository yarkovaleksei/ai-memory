import DeleteIcon from "@mui/icons-material/DeleteOutlined";
import FolderIcon from "@mui/icons-material/FolderOpenOutlined";
import {
  Alert,
  Button,
  Card,
  CardContent,
  Chip,
  Divider,
  FormControlLabel,
  IconButton,
  MenuItem,
  Stack,
  Switch,
  TextField,
  Typography,
} from "@mui/material";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
import { Title, useNotify, useRefresh, useTranslate } from "react-admin";
import { SettingsService, type Settings, type SettingsView } from "../api/bindings";
import { call } from "../api/errors";
import { useErrorText } from "../items/text";
import { Rich } from "../layout/Rich";
import { AppUpdates } from "./AppUpdates";

const SETTINGS_KEY = ["settings"];

// Экран «Настройки»: каталоги профилей Claude, корень и поколение Codex. Пишет
// только config.toml самого ai-memory; память агентов не меняется. После
// сохранения память перечитывается без перезапуска.
export const SettingsPage = () => {
  const translate = useTranslate();
  const errorText = useErrorText();
  const { data, error } = useQuery<SettingsView>({
    queryKey: SETTINGS_KEY,
    queryFn: () => call(() => SettingsService.Get()),
  });
  return (
    <Card sx={{ mt: 2 }}>
      <Title title={translate("aim.settings.title")} />
      <CardContent>
        <Stack spacing={2}>
          <Typography variant="h5" component="h1">
            {translate("aim.settings.title")}
          </Typography>
          {error && <Alert severity="error">{errorText(error)}</Alert>}
          {!data && !error && <Typography>{translate("aim.settings.loading")}</Typography>}
          {/* key: после сохранения форма начинается с того, что записано в файл */}
          {data && <SettingsForm key={JSON.stringify(data.settings)} view={data} />}
          <Divider />
          <AppUpdates />
        </Stack>
      </CardContent>
    </Card>
  );
};

const SettingsForm = ({ view }: { view: SettingsView }) => {
  const translate = useTranslate();
  const notify = useNotify();
  const refresh = useRefresh();
  const errorText = useErrorText();
  const queryClient = useQueryClient();
  const [st, setSt] = useState<Settings>(view.settings);
  const [manual, setManual] = useState("");
  const [saving, setSaving] = useState(false);
  const extra = st.extraClaudeProfiles ?? [];

  const addProfile = (dir: string) => {
    const d = dir.trim();
    if (d && !extra.includes(d)) setSt({ ...st, extraClaudeProfiles: [...extra, d] });
  };
  const pick = async (apply: (dir: string) => void) => {
    try {
      const dir = await call(() => SettingsService.PickDirectory(translate("aim.settings.pickTitle")));
      if (dir) apply(dir);
    } catch (err) {
      notify(errorText(err), { type: "error" });
    }
  };
  const save = async () => {
    setSaving(true);
    try {
      const next = await call(() => SettingsService.Save(st));
      queryClient.setQueryData(SETTINGS_KEY, next);
      notify("aim.settings.saved", { type: "success" });
      refresh();
    } catch (err) {
      notify(errorText(err), { type: "error", multiLine: true });
    } finally {
      setSaving(false);
    }
  };

  return (
    <Stack spacing={3}>
      <Alert severity="info">
        <Rich text={translate("aim.settings.intro", { file: view.configFile })} />
      </Alert>

      <Stack spacing={1}>
        <Typography variant="h6" component="h2">
          {translate("aim.settings.claude.title")}
        </Typography>
        {view.lockedClaudeProfiles && (
          <Alert severity="warning">
            <Rich text={translate("aim.settings.claude.locked")} />
          </Alert>
        )}
        <Typography variant="body2" color="text.secondary">
          <Rich text={translate("aim.settings.claude.help")} />
        </Typography>
        {(view.defaultClaudeProfiles ?? []).map((p) => (
          <Stack key={p} direction="row" spacing={1} sx={{ alignItems: "center" }}>
            <Typography sx={{ fontFamily: "monospace", wordBreak: "break-all" }}>{p}</Typography>
            <Chip size="small" variant="outlined" label={translate("aim.settings.default")} />
          </Stack>
        ))}
        {extra.map((p) => (
          <Stack key={p} direction="row" spacing={1} sx={{ alignItems: "center" }}>
            <Typography sx={{ fontFamily: "monospace", wordBreak: "break-all" }}>{p}</Typography>
            <IconButton
              aria-label={translate("aim.settings.claude.remove", { dir: p })}
              onClick={() => setSt({ ...st, extraClaudeProfiles: extra.filter((x) => x !== p) })}
            >
              <DeleteIcon fontSize="small" />
            </IconButton>
          </Stack>
        ))}
        <DirField
          label={translate("aim.settings.claude.manual")}
          value={manual}
          onChange={setManual}
          onPick={() => void pick(setManual)}
        >
          <Button
            variant="outlined"
            disabled={!manual.trim()}
            onClick={() => {
              addProfile(manual);
              setManual("");
            }}
          >
            {translate("aim.settings.claude.add")}
          </Button>
        </DirField>
      </Stack>

      <Divider />

      <Stack spacing={1.5}>
        <Typography variant="h6" component="h2">
          {translate("aim.settings.codex.title")}
        </Typography>
        {view.lockedCodexHome && (
          <Alert severity="warning">
            <Rich text={translate("aim.settings.codex.locked")} />
          </Alert>
        )}
        <DirField
          label={translate("aim.settings.codex.home")}
          placeholder={view.defaultCodexHome}
          helperText={
            <Rich text={translate("aim.settings.codex.homeHelp", { dir: view.defaultCodexHome })} />
          }
          value={st.codexHome}
          onChange={(codexHome) => setSt({ ...st, codexHome })}
          onPick={() => void pick((dir) => setSt({ ...st, codexHome: dir }))}
        />
        <TextField
          select
          label={translate("aim.settings.codex.generation")}
          value={st.codexGeneration}
          onChange={(e) => setSt({ ...st, codexGeneration: e.target.value })}
          // Пустое значение — «определять автоматически»: его подпись должна быть видна.
          slotProps={{ select: { displayEmpty: true }, inputLabel: { shrink: true } }}
        >
          <MenuItem value="">
            <Rich
              text={translate("aim.settings.codex.auto", {
                generation: view.codexGeneration,
                reason: translate(`aim.settings.reasons.${view.generationReason}`, {
                  _: view.generationReason,
                }),
              })}
            />
          </MenuItem>
          <MenuItem value="v1">
            <Rich text={translate("aim.settings.codex.v1")} />
          </MenuItem>
          <MenuItem value="v2">
            <Rich text={translate("aim.settings.codex.v2")} />
          </MenuItem>
        </TextField>
        <FormControlLabel
          control={
            <Switch
              checked={st.showCodexInactive}
              onChange={(e) => setSt({ ...st, showCodexInactive: e.target.checked })}
            />
          }
          label={translate("aim.settings.codex.showInactive")}
        />
      </Stack>

      <Stack direction="row">
        <Button variant="contained" disabled={saving} onClick={() => void save()}>
          {translate("ra.action.save")}
        </Button>
      </Stack>
    </Stack>
  );
};

interface DirFieldProps {
  label: string;
  value: string;
  onChange: (value: string) => void;
  onPick: () => void;
  placeholder?: string;
  helperText?: ReactNode;
  children?: ReactNode; // ещё действия справа, например «Добавить»
}

// DirField — путь к каталогу: поле и справа «Выбрать каталог», который
// вписывает выбранное в поле. Один вид для всех каталогов на экране.
const DirField = ({ label, value, onChange, onPick, placeholder, helperText, children }: DirFieldProps) => {
  const translate = useTranslate();
  return (
    <Stack direction="row" spacing={1} sx={{ alignItems: "flex-start" }}>
      <TextField
        label={label}
        placeholder={placeholder}
        helperText={helperText}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
      <Button variant="outlined" startIcon={<FolderIcon />} onClick={onPick} sx={{ flexShrink: 0 }}>
        {translate("aim.settings.pick")}
      </Button>
      {children}
    </Stack>
  );
};
