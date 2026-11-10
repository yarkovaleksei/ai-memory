import { Chip } from "@mui/material";
import { useTranslate } from "react-admin";

// Цвет агента одинаков в дереве, списке и просмотре: так проще различать,
// чья запись, не читая подпись.
const agentColor = (agent: string) => (agent === "claude" ? "warning" : "info");

export const AgentBadge = ({ agent }: { agent: string }) => {
  const translate = useTranslate();
  return (
    <Chip
      component="span"
      size="small"
      variant="outlined"
      color={agentColor(agent)}
      label={translate(`aim.agents.${agent}`, { _: agent })}
      sx={{ height: 18, fontSize: "0.7rem" }}
    />
  );
};

export const TypeBadge = ({ type }: { type: string }) => {
  const translate = useTranslate();
  if (!type) return null;
  return (
    <Chip
      component="span"
      size="small"
      label={translate(`aim.types.${type}`, { _: type })}
      sx={{ height: 18, fontSize: "0.7rem" }}
    />
  );
};
