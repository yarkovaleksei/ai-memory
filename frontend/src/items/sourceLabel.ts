import { useTranslate } from "react-admin";
import type { Source } from "../api/bindings";

// useSourceLabel — подпись хранилища. У хранилищ инструкций и навыков (ADR-019)
// своя подпись: у проектных файлов нет ни имени, ни общего корня.
export const useSourceLabel = () => {
  const translate = useTranslate();
  return (s: Pick<Source, "agent" | "label" | "root" | "files">) => {
    if (!s.files) return s.label;
    const agent = translate(`aim.agents.${s.agent}`, { _: s.agent });
    return s.root
      ? translate("aim.sources.files", { label: s.label, agent })
      : translate("aim.sources.projectFiles", { agent });
  };
};
