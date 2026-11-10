import { MergeView } from "@codemirror/merge";
import { EditorState } from "@codemirror/state";
import { EditorView, lineNumbers } from "@codemirror/view";
import { Box, useTheme } from "@mui/material";
import { useEffect, useRef } from "react";
import type { Change } from "../api/bindings";
import { baseExtensions } from "./codemirror";

// Diff одного файла плана (ADR-013): для правки — две колонки «было / станет»
// с подсветкой изменений и свёрнутыми неизменёнными участками; для нового или
// удаляемого файла — одна колонка с его содержимым.
export const DiffView = ({ change }: { change: Change }) => {
  const theme = useTheme();
  const host = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const parent = host.current;
    if (!parent) return;
    const extensions = [
      ...baseExtensions(theme),
      lineNumbers(),
      EditorView.editable.of(false),
      EditorState.readOnly.of(true),
    ];
    if (change.beforeExists && change.afterExists) {
      const mv = new MergeView({
        parent,
        a: { doc: change.before, extensions },
        b: { doc: change.after, extensions },
        collapseUnchanged: { margin: 3, minSize: 6 },
        gutter: true,
      });
      return () => mv.destroy();
    }
    const view = new EditorView({
      parent,
      state: EditorState.create({ doc: change.afterExists ? change.after : change.before, extensions }),
    });
    return () => view.destroy();
  }, [change, theme]);

  const tint = change.afterExists ? (change.beforeExists ? undefined : "success.main") : "error.main";
  return (
    <Box
      ref={host}
      sx={{
        border: 1,
        borderColor: tint ?? "divider",
        borderLeftWidth: tint ? 4 : 1,
        borderRadius: 1,
        overflow: "auto",
        maxHeight: 420,
      }}
    />
  );
};
