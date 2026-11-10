import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { Box, useTheme } from "@mui/material";
import { basicSetup } from "codemirror";
import { useEffect, useRef } from "react";
import { baseExtensions } from "./codemirror";

export interface CodeEditorProps {
  value: string;
  onChange: (value: string) => void;
  label: string; // доступное имя поля: редактор — contenteditable, а не <textarea>
  minHeight?: number;
}

// Редактор Markdown на CodeMirror 6 без обёртки для React (ADR-013). Состояние
// документа живёт в EditorView; наружу уходит каждое изменение, а внешнее
// значение вписывается, только если отличается (иначе сбивался бы курсор).
export const CodeEditor = ({ value, onChange, label, minHeight = 240 }: CodeEditorProps) => {
  const theme = useTheme();
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const onChangeRef = useRef(onChange);
  const valueRef = useRef(value);

  useEffect(() => {
    onChangeRef.current = onChange;
    valueRef.current = value;
  });

  useEffect(() => {
    if (!host.current) return;
    const v = new EditorView({
      parent: host.current,
      state: EditorState.create({
        doc: valueRef.current,
        extensions: [
          basicSetup,
          ...baseExtensions(theme),
          EditorView.contentAttributes.of({ "aria-label": label }),
          EditorView.updateListener.of((u) => {
            if (u.docChanged) onChangeRef.current(u.state.doc.toString());
          }),
        ],
      }),
    });
    view.current = v;
    return () => {
      v.destroy();
      view.current = null;
    };
  }, [theme, label]);

  useEffect(() => {
    const v = view.current;
    if (v && v.state.doc.toString() !== value) {
      v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: value } });
    }
  }, [value]);

  return (
    <Box
      ref={host}
      sx={{
        border: 1,
        borderColor: "divider",
        borderRadius: 1,
        overflow: "hidden",
        "& .cm-editor": { minHeight },
        "& .cm-scroller": { minHeight },
      }}
    />
  );
};
