import { useCallback } from "react";
import { useTranslate } from "react-admin";
import type { Warning } from "../api/bindings";
import type { AppError } from "../api/errors";

// Маркеры подсветки в сниппетах — те же управляющие символы, что в internal/index.
export const MARK_START = "\u0002";
export const MARK_END = "\u0003";

export interface SnippetPart {
  text: string;
  hit: boolean;
}

// splitSnippet режет сниппет FTS на куски: совпадения помечены hit.
export const splitSnippet = (snippet: string): SnippetPart[] => {
  const parts: SnippetPart[] = [];
  let hit = false;
  let text = "";
  const flush = () => {
    if (text) parts.push({ text, hit });
    text = "";
  };
  for (const ch of snippet) {
    if (ch === MARK_START || ch === MARK_END) {
      flush();
      hit = ch === MARK_START;
    } else {
      text += ch;
    }
  }
  flush();
  return parts;
};

// useWarningText собирает текст предупреждения по коду из Go; неизвестный код
// показывается как есть, а не пропадает.
export const useWarningText = () => {
  const translate = useTranslate();
  return useCallback(
    (w: Warning) =>
      translate(`aim.warnings.${w.code}`, {
        ...(w.params ?? {}),
        _: translate("aim.warnings.unknown", { code: w.code }),
      }),
    [translate],
  );
};

export const useErrorText = () => {
  const translate = useTranslate();
  return useCallback(
    (err: unknown) => {
      const e = err as Partial<AppError> | null;
      const params = e?.params ?? {};
      const message = e?.message ?? "";
      return translate(message, {
        ...params,
        _: translate("aim.errors.unknown", { detail: params.detail ?? message }),
      });
    },
    [translate],
  );
};
