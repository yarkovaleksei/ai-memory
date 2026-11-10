import { markdown } from "@codemirror/lang-markdown";
import type { Extension } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import type { Theme } from "@mui/material";

// Общие расширения CodeMirror (ADR-013): Markdown, перенос строк и тема из
// палитры MUI, чтобы редактор и diff выглядели частью окна в обеих темах.
export const baseExtensions = (theme: Theme): Extension[] => [
  markdown(),
  EditorView.lineWrapping,
  EditorView.theme(
    {
      "&": {
        fontSize: "13px",
        color: theme.palette.text.primary,
        backgroundColor: theme.palette.background.paper,
      },
      ".cm-content": { fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace" },
      ".cm-gutters": {
        backgroundColor: theme.palette.background.default,
        color: theme.palette.text.secondary,
        borderRight: `1px solid ${theme.palette.divider}`,
      },
      "&.cm-focused": { outline: "none" },
      ".cm-cursor": { borderLeftColor: theme.palette.text.primary },
    },
    { dark: theme.palette.mode === "dark" },
  ),
];
