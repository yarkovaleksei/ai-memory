import { Alert, Box, Button, LinearProgress, Stack, Typography } from "@mui/material";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { useNotify, useTranslate } from "react-admin";
import { Browser, Events, UpdatesService, UpdateState, UpdateStatus, type UpdateView } from "../api/bindings";
import { call } from "../api/errors";
import { useErrorText } from "../items/text";
import { Rich } from "../layout/Rich";

export const UPDATES_KEY = ["updates"];

const RELEASES_URL = "https://github.com/yarkovaleksei/ai-memory/releases";

// Раздел «Обновление ai-memory» (ADR-031): в сеть — только по кнопке.
// Скачивание идёт в Go в фоне, прогресс и итог приходят событием
// update:status и кладутся в кэш запроса.
export const AppUpdates = () => {
  const translate = useTranslate();
  const notify = useNotify();
  const errorText = useErrorText();
  const queryClient = useQueryClient();
  const [installing, setInstalling] = useState(false);
  const { data: st } = useQuery<UpdateView>({
    queryKey: UPDATES_KEY,
    queryFn: () => call(() => UpdatesService.Status()),
  });
  useEffect(
    () => Events.On(UpdateStatus, (ev) => queryClient.setQueryData(UPDATES_KEY, ev.data)),
    [queryClient],
  );

  const run = async (fn: () => Promise<UpdateView>) => {
    try {
      queryClient.setQueryData(UPDATES_KEY, await call(fn));
    } catch (err) {
      notify(errorText(err), { type: "error", multiLine: true });
    }
  };
  const install = async () => {
    setInstalling(true);
    try {
      await call(() => UpdatesService.Install());
    } catch (err) {
      notify(errorText(err), { type: "error", multiLine: true });
      queryClient.setQueryData(UPDATES_KEY, await call(() => UpdatesService.Status()));
    } finally {
      setInstalling(false);
    }
  };

  if (!st) return null;
  const size = (bytes: number) =>
    new Intl.NumberFormat("ru", { style: "unit", unit: "megabyte", maximumFractionDigits: 1 }).format(
      bytes / 1e6,
    );
  const failure = st.error
    ? translate(`aim.errors.${st.error.code}`, {
        ...(st.error.params ?? {}),
        _: translate("aim.errors.unknown", { detail: st.error.code }),
      })
    : "";
  const busy = st.state === UpdateState.StateChecking || st.state === UpdateState.StateDownloading;
  const notes = st.notesUrl ? (
    <Button onClick={() => void Browser.OpenURL(st.notesUrl ?? RELEASES_URL)}>
      {translate("aim.updates.notes")}
    </Button>
  ) : null;

  return (
    <Stack spacing={1.5}>
      <Typography variant="h6" component="h2">
        {translate("aim.updates.title")}
      </Typography>
      <Typography variant="body2" color="text.secondary">
        {translate("aim.updates.current", { version: st.current })}
      </Typography>

      {st.state === UpdateState.StateUnsupported && (
        <Alert
          severity="info"
          action={
            <Button color="inherit" onClick={() => void Browser.OpenURL(RELEASES_URL)}>
              {translate("aim.updates.releases")}
            </Button>
          }
        >
          {translate("aim.updates.unsupported")}
        </Alert>
      )}
      {st.state === UpdateState.StateCurrent && (
        <Alert severity="success">{translate("aim.updates.upToDate", { version: st.latest })}</Alert>
      )}
      {failure && <Alert severity="error">{failure}</Alert>}
      {st.state === UpdateState.StateAvailable && (
        <Alert severity="info">
          {translate("aim.updates.available", { version: st.latest, size: size(st.size ?? 0) })}
        </Alert>
      )}
      {st.state === UpdateState.StateDownloading && (
        <Stack spacing={0.5}>
          <Typography variant="body2">
            {translate("aim.updates.downloading", {
              received: size(st.received ?? 0),
              size: size(st.size ?? 0),
            })}
          </Typography>
          <LinearProgress
            variant="determinate"
            value={st.size ? Math.min(100, ((st.received ?? 0) / st.size) * 100) : 0}
            aria-label={translate("aim.updates.progress")}
          />
        </Stack>
      )}
      {st.state === UpdateState.StateReady && (
        <Alert severity="success">
          <Stack spacing={1}>
            <Rich
              text={translate(`aim.updates.ready.${st.install}`, { version: st.latest, file: st.package })}
            />
            {st.command && (
              <Box
                component="code"
                sx={{ fontFamily: "monospace", wordBreak: "break-all", userSelect: "all" }}
              >
                {st.command}
              </Box>
            )}
          </Stack>
        </Alert>
      )}

      <Stack direction="row" spacing={1} sx={{ flexWrap: "wrap" }}>
        {st.state !== UpdateState.StateUnsupported && st.state !== UpdateState.StateReady && (
          <Button variant="outlined" disabled={busy} onClick={() => void run(() => UpdatesService.Check())}>
            {translate(st.state === UpdateState.StateChecking ? "aim.updates.checking" : "aim.updates.check")}
          </Button>
        )}
        {(st.state === UpdateState.StateAvailable ||
          (st.state === UpdateState.StateFailed && st.package)) && (
          <Button variant="contained" onClick={() => void call(() => UpdatesService.Download())}>
            {translate(st.state === UpdateState.StateFailed ? "aim.updates.retry" : "aim.updates.download")}
          </Button>
        )}
        {st.state === UpdateState.StateDownloading && (
          <Button onClick={() => void call(() => UpdatesService.Cancel())}>
            {translate("aim.updates.cancel")}
          </Button>
        )}
        {st.state === UpdateState.StateReady && (
          <Button variant="contained" disabled={installing} onClick={() => void install()}>
            {translate(`aim.updates.install.${st.install}`)}
          </Button>
        )}
        {notes}
      </Stack>
    </Stack>
  );
};
