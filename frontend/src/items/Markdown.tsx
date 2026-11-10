import { Box, Link } from "@mui/material";
import { useMemo, type ReactNode } from "react";
import ReactMarkdown, { defaultUrlTransform, type Components } from "react-markdown";
import { useTranslate } from "react-admin";
import remarkGfm from "remark-gfm";
import { Browser, type Link as ItemLink } from "../api/bindings";

// Схема ссылок на другие записи внутри текста: aim-item:<id>.
const ITEM_SCHEME = "aim-item:";

const escapeRegExp = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

// linkify превращает ссылки, которые нашёл Go, в markdown-ссылки на записи:
// [[slug]] у фактов Claude и пути rollout_summaries/… в сводке Codex. Неразрешённые
// [[slug]] остаются ссылкой без цели — их видно, но открыть нечего.
export const linkify = (body: string, links: ItemLink[]): string => {
  let out = body;
  for (const l of links) {
    const target = l.targetId ? `${ITEM_SCHEME}${l.targetId}` : `${ITEM_SCHEME}`;
    out = out.split(`[[${l.text}]]`).join(`[${l.text}](${target})`);
    if (l.text.includes("/") && l.targetId) {
      // Голый путь, который ещё не внутри ссылки или кода.
      const bare = new RegExp(`(?<![\\[(\`/\\w])${escapeRegExp(l.text)}(?![\\w\`\\]])`, "g");
      out = out.replace(bare, `[${l.text}](${target})`);
    }
  }
  return out;
};

interface MarkdownProps {
  body: string;
  links: ItemLink[];
  onOpenItem: (id: string) => void;
}

export const Markdown = ({ body, links, onOpenItem }: MarkdownProps) => {
  const translate = useTranslate();
  const text = useMemo(() => linkify(body, links), [body, links]);
  // Относительные ссылки из MEMORY.md ([Заголовок](файл.md)) Go тоже разрешил.
  const byHref = useMemo(() => new Map(links.map((l) => [l.text, l.targetId])), [links]);

  const components = useMemo<Components>(
    () => ({
      a: ({ href = "", children }: { href?: string; children?: ReactNode }) => {
        const target = href.startsWith(ITEM_SCHEME) ? href.slice(ITEM_SCHEME.length) : byHref.get(href);
        if (target !== undefined) {
          if (!target) {
            return (
              <Box
                component="span"
                title={translate("aim.preview.brokenLink", {
                  text: typeof children === "string" ? children : href,
                })}
                sx={{ color: "warning.main", textDecoration: "underline dotted" }}
              >
                {children}
              </Box>
            );
          }
          return (
            <Link
              component="button"
              type="button"
              onClick={() => onOpenItem(target)}
              sx={{ verticalAlign: "baseline" }}
            >
              {children}
            </Link>
          );
        }
        // Внешние ссылки открываются в системном браузере, а не в окне приложения.
        const external = /^https?:\/\//i.test(href);
        return (
          <Link
            href={href}
            onClick={(e) => {
              e.preventDefault();
              if (external) void Browser.OpenURL(href);
            }}
          >
            {children}
          </Link>
        );
      },
    }),
    [byHref, onOpenItem, translate],
  );

  return (
    <Box
      sx={{
        "& pre": { overflowX: "auto", p: 1, bgcolor: "action.hover", borderRadius: 1 },
        "& code": { fontFamily: "monospace", fontSize: "0.9em" },
        "& table": { borderCollapse: "collapse" },
        "& th, & td": { border: 1, borderColor: "divider", px: 1 },
        wordBreak: "break-word",
      }}
    >
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={components}
        urlTransform={(url) => (url.startsWith(ITEM_SCHEME) ? url : defaultUrlTransform(url))}
      >
        {text}
      </ReactMarkdown>
    </Box>
  );
};
