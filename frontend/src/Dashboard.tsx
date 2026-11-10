import { Alert, Card, CardContent, List, ListItem, ListItemText, Stack, Typography } from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { Title, useGetList, useTranslate } from "react-admin";
import { InfoService, type AppInfo, type Source } from "./api/bindings";
import { useWarningText } from "./items/text";

// Экран «О программе»: версия, каталоги ai-memory и найденные хранилища памяти
// с их предупреждениями (например, autoMemoryDirectory в настройках Claude).
export const Dashboard = () => {
  const translate = useTranslate();
  const { data, error, isPending } = useQuery<AppInfo, Error>({
    queryKey: ["app", "info"],
    queryFn: () => InfoService.Info(),
  });

  return (
    <Card sx={{ mt: 2 }}>
      <Title title={translate("aim.dashboard.title")} />
      <CardContent>
        <Stack spacing={2}>
          <div>
            <Typography variant="h5" component="h1">
              {translate("aim.dashboard.title")}
            </Typography>
            <Typography color="text.secondary">{translate("aim.dashboard.subtitle")}</Typography>
          </div>
          {isPending && <Typography>{translate("aim.dashboard.loading")}</Typography>}
          {error && (
            <Alert severity="error">{translate("aim.dashboard.error", { message: error.message })}</Alert>
          )}
          {data && <InfoList info={data} />}
          <SourceList />
        </Stack>
      </CardContent>
    </Card>
  );
};

const InfoList = ({ info }: { info: AppInfo }) => {
  const translate = useTranslate();
  const profiles = info.claudeProfiles ?? [];
  return (
    <>
      <Typography variant="body2" color="text.secondary">
        {translate("aim.dashboard.version", { version: info.version })}
      </Typography>
      <List dense disablePadding>
        <ListItem disableGutters>
          <ListItemText
            primary={translate("aim.dashboard.sections.claude")}
            secondary={
              profiles.length > 0 ? profiles.join("\n") : translate("aim.dashboard.noClaudeProfiles")
            }
            slotProps={{ secondary: { sx: { whiteSpace: "pre-line" } } }}
          />
        </ListItem>
        <PathItem label={translate("aim.dashboard.sections.codex")} path={info.codexHome} />
        <PathItem label={translate("aim.dashboard.sections.config")} path={info.configDir} />
        <PathItem label={translate("aim.dashboard.sections.data")} path={info.dataDir} />
      </List>
    </>
  );
};

const PathItem = ({ label, path }: { label: string; path: string }) => (
  <ListItem disableGutters>
    <ListItemText primary={label} secondary={path} />
  </ListItem>
);

const SourceList = () => {
  const translate = useTranslate();
  const warningText = useWarningText();
  const { data = [] } = useGetList<Source & { id: string }>("sources", {
    pagination: { page: 1, perPage: 1000 },
  });
  if (data.length === 0) return null;
  return (
    <div>
      <Typography variant="subtitle1" component="h2">
        {translate("aim.dashboard.sections.sources")}
      </Typography>
      <List dense disablePadding>
        {data.map((s) => (
          <ListItem key={s.id} disableGutters sx={{ display: "block" }}>
            <ListItemText
              primary={s.readOnly ? `${s.label} · ${translate("aim.dashboard.readOnlySource")}` : s.label}
              secondary={s.root}
            />
            {(s.warnings ?? []).map((w, i) => (
              <Alert key={`${w.code}-${i}`} severity="warning" sx={{ mt: 0.5 }}>
                {warningText(w)}
              </Alert>
            ))}
          </ListItem>
        ))}
      </List>
    </div>
  );
};
