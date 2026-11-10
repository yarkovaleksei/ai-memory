import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Radio,
  RadioGroup,
  Stack,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslate } from "react-admin";
import { PlansService, type Item, type SkillTarget } from "../api/bindings";
import { call } from "../api/errors";
import { useErrorText } from "../items/text";
import { useRunPlan } from "./plans";

// Навык ставится в того агента, которому он не принадлежит.
export const otherAgent = (it: Pick<Item, "agent">) => (it.agent === "claude" ? "codex" : "claude");

// SkillDialog — установка навыка в другого агента копией или симлинком
// (ADR-023). Поля frontmatter, которых тот агент не понимает, перечисляет
// окно плана; Go получает только ID навыка и ключ цели.
export const SkillDialog = ({ item, onClose }: { item: Item; onClose: () => void }) => {
  const translate = useTranslate();
  const errorText = useErrorText();
  const agent = translate(`aim.agents.${otherAgent(item)}`);
  const targets = useQuery<SkillTarget[]>({
    queryKey: ["plans", "skillTargets", item.id],
    queryFn: async () => (await call(() => PlansService.SkillTargets(item.id))) ?? [],
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
  return (
    <Dialog open onClose={onClose} maxWidth="sm" fullWidth aria-labelledby="skill-title">
      <DialogTitle id="skill-title">{translate("aim.skill.title", { title: item.title, agent })}</DialogTitle>
      {targets.error ? (
        <DialogContent>
          <Alert severity="error">{errorText(targets.error)}</Alert>
        </DialogContent>
      ) : !targets.data ? (
        <DialogContent>
          <Typography color="text.secondary">{translate("aim.preview.loading")}</Typography>
        </DialogContent>
      ) : (
        <SkillForm item={item} targets={targets.data} onClose={onClose} />
      )}
    </Dialog>
  );
};

const SkillForm = ({
  item,
  targets,
  onClose,
}: {
  item: Item;
  targets: SkillTarget[];
  onClose: () => void;
}) => {
  const translate = useTranslate();
  const runPlan = useRunPlan();
  const free = targets.filter((t) => !t.exists);
  const [key, setKey] = useState((free.find((t) => t.recommended) ?? free[0])?.key ?? "");
  const [mode, setMode] = useState<"copy" | "link">("copy");
  const target = targets.find((t) => t.key === key);
  return (
    <>
      <DialogContent dividers>
        <Stack spacing={2}>
          {targets.length === 0 ? (
            <Alert severity="info">{translate("aim.skill.noTargets")}</Alert>
          ) : (
            <RadioGroup
              value={key}
              onChange={(e) => setKey(e.target.value)}
              aria-label={translate("aim.skill.target")}
            >
              {targets.map((t) => (
                <FormControlLabel
                  key={t.key}
                  value={t.key}
                  disabled={t.exists}
                  control={<Radio />}
                  label={
                    <Stack>
                      <span>
                        {translate(`aim.skill.targets.${t.kind}`, { label: t.label })}
                        {t.recommended && ` — ${translate("aim.skill.recommended")}`}
                        {t.exists && ` — ${translate("aim.skill.exists")}`}
                      </span>
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        sx={{ fontFamily: "monospace", wordBreak: "break-all" }}
                      >
                        {t.dir}
                      </Typography>
                    </Stack>
                  }
                />
              ))}
            </RadioGroup>
          )}
          <ToggleButtonGroup
            exclusive
            value={mode}
            onChange={(_, v: "copy" | "link" | null) => v && setMode(v)}
            aria-label={translate("aim.skill.mode")}
          >
            <ToggleButton value="copy">{translate("aim.skill.copy")}</ToggleButton>
            <ToggleButton value="link">{translate("aim.skill.link")}</ToggleButton>
          </ToggleButtonGroup>
          <Alert severity={mode === "link" ? "warning" : "info"}>
            {translate(mode === "link" ? "aim.skill.linkHelp" : "aim.skill.copyHelp")}
          </Alert>
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{translate("ra.action.cancel")}</Button>
        <Button
          variant="contained"
          disabled={!target || target.exists}
          onClick={() =>
            void runPlan(() => PlansService.InstallSkill(item.id, key, mode === "link"), {
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
