import {
  Alert,
  Button,
  Card,
  CardContent,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { Title, useTranslate } from "react-admin";
import { HistoryService, type Entry } from "../api/bindings";
import { call } from "../api/errors";
import { useEditStatus, useOpText, useRunPlan } from "../edit/plans";
import { useErrorText } from "../items/text";

// Экран истории: операции из репозитория бэкапов, новые первыми. Откат — тоже
// план с diff; он сам попадает в историю, и его можно откатить.
export const HistoryPage = () => {
  const translate = useTranslate();
  const errorText = useErrorText();
  const { data: status } = useEditStatus();
  const { data, error, isPending } = useQuery<Entry[]>({
    queryKey: ["history"],
    queryFn: async () => (await call(() => HistoryService.List())) ?? [],
  });

  return (
    <Card sx={{ mt: 2 }}>
      <Title title={translate("aim.history.title")} />
      <CardContent>
        <Stack spacing={2}>
          <div>
            <Typography variant="h5" component="h1">
              {translate("aim.history.title")}
            </Typography>
            <Typography color="text.secondary">{translate("aim.history.subtitle")}</Typography>
          </div>
          {status && !status.enabled && (
            <Alert severity="warning">{translate(`aim.errors.${status.reason}`, { _: status.reason })}</Alert>
          )}
          {error && <Alert severity="error">{errorText(error)}</Alert>}
          {isPending && <Typography>{translate("aim.history.loading")}</Typography>}
          {data && data.length === 0 && <Typography>{translate("aim.history.empty")}</Typography>}
          {data && data.length > 0 && <HistoryTable entries={data} canWrite={!!status?.enabled} />}
        </Stack>
      </CardContent>
    </Card>
  );
};

const baseName = (p: string) => p.split(/[\\/]/).pop() ?? p;

const HistoryTable = ({ entries, canWrite }: { entries: Entry[]; canWrite: boolean }) => {
  const translate = useTranslate();
  const opText = useOpText();
  const runPlan = useRunPlan();
  return (
    <Table size="small">
      <TableHead>
        <TableRow>
          <TableCell>{translate("aim.history.columns.time")}</TableCell>
          <TableCell>{translate("aim.history.columns.op")}</TableCell>
          <TableCell>{translate("aim.history.columns.files")}</TableCell>
          <TableCell />
        </TableRow>
      </TableHead>
      <TableBody>
        {entries.map((e) => (
          <TableRow key={e.commit}>
            <TableCell sx={{ whiteSpace: "nowrap" }}>{new Date(e.time).toLocaleString()}</TableCell>
            <TableCell>{opText(e.op)}</TableCell>
            <TableCell>
              <Stack>
                {(e.paths ?? []).map((p) => (
                  <Tooltip key={p} title={p}>
                    <Typography variant="body2" sx={{ fontFamily: "monospace" }}>
                      {baseName(p)}
                    </Typography>
                  </Tooltip>
                ))}
              </Stack>
            </TableCell>
            <TableCell align="right">
              <Button
                size="small"
                disabled={!canWrite}
                onClick={() => void runPlan(() => HistoryService.Revert(e.commit))}
              >
                {translate("aim.history.revert")}
              </Button>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
};
