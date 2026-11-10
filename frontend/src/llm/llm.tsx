import { Alert, Box, Button, CircularProgress, Stack, Typography } from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { useTranslate } from "react-admin";
import { LLMService, type Agent } from "../api/bindings";
import { call } from "../api/errors";
import { useErrorText } from "../items/text";

// Агенты, чьи CLI найдены в PATH. Набор не меняется, пока окно открыто.
export const useLLMAgents = () =>
  useQuery<Agent[]>({
    queryKey: ["llm", "agents"],
    queryFn: async () => (await call(() => LLMService.Agents())) ?? [],
    staleTime: Infinity,
  });

type Cancellable<T> = Promise<T> & { cancel?: () => void };

// useLLMCall — запрос к модели, который можно остановить: «Остановить» или
// закрытие окна отменяет промис Wails, а Go — контекст и процесс CLI.
export const useLLMCall = <T,>() => {
  const [running, setRunning] = useState<Agent | null>(null);
  const [error, setError] = useState<unknown>(null);
  const current = useRef<Cancellable<T> | null>(null);
  useEffect(() => () => current.current?.cancel?.(), []);

  const run = async (agent: Agent, start: (agent: Agent) => Cancellable<T>): Promise<T | undefined> => {
    const p = start(agent);
    current.current = p;
    setRunning(agent);
    setError(null);
    try {
      return await call(() => p);
    } catch (err) {
      if (current.current === p) setError(err); // остановленный запрос — не ошибка
      return undefined;
    } finally {
      if (current.current === p) {
        current.current = null;
        setRunning(null);
      }
    }
  };
  const stop = () => {
    const p = current.current;
    current.current = null;
    setRunning(null);
    p?.cancel?.();
  };
  return { running, error, run, stop };
};

interface LLMActionsProps {
  label: string; // ключ словаря кнопки с %{agent}
  busy: string; // ключ словаря строки ожидания с %{agent}
  running: Agent | null;
  error: unknown;
  disabled?: boolean;
  onRun: (agent: Agent) => void;
  onStop: () => void;
}

// LLMActions — кнопка на каждого найденного агента, ожидание с «Остановить»
// и ошибка последнего запроса.
export const LLMActions = ({ label, busy, running, error, disabled, onRun, onStop }: LLMActionsProps) => {
  const translate = useTranslate();
  const errorText = useErrorText();
  const { data: agents } = useLLMAgents();
  const name = (a: Agent) => translate(`aim.llm.agents.${a}`, { _: a });
  if (agents && agents.length === 0) {
    return <Typography color="text.secondary">{translate("aim.llm.noAgents")}</Typography>;
  }
  return (
    <Stack spacing={1}>
      {running ? (
        <Stack direction="row" spacing={1.5} sx={{ alignItems: "center" }}>
          <CircularProgress size={20} />
          <Typography>{translate(busy, { agent: name(running) })}</Typography>
          <Box sx={{ flex: 1 }} />
          <Button size="small" onClick={onStop}>
            {translate("aim.llm.stop")}
          </Button>
        </Stack>
      ) : (
        <Stack direction="row" spacing={1} sx={{ flexWrap: "wrap", rowGap: 1 }}>
          {(agents ?? []).map((a) => (
            <Button key={a} size="small" variant="outlined" disabled={disabled} onClick={() => onRun(a)}>
              {translate(label, { agent: name(a) })}
            </Button>
          ))}
        </Stack>
      )}
      {!!error && <Alert severity="error">{errorText(error)}</Alert>}
    </Stack>
  );
};
