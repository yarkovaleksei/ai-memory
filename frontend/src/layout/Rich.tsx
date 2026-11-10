import { Box } from "@mui/material";
import { Fragment } from "react";

// Rich показывает строку словаря, где пути, команды и переменные окружения
// взяты в обратные кавычки: `~/.claude`. Они выделяются моноширинным шрифтом
// и фоном, чтобы не сливаться с текстом. Строки с кавычками выводятся только
// через Rich: в уведомлениях react-admin кавычки остались бы как есть.
export const Rich = ({ text }: { text: string }) => (
  <>
    {text.split("`").map((part, i) =>
      i % 2 === 1 ? (
        <Box
          key={i}
          component="code"
          sx={{
            fontFamily: "monospace",
            fontSize: "0.9em",
            px: 0.5,
            py: 0.125,
            borderRadius: 0.5,
            bgcolor: "action.hover",
            wordBreak: "break-all",
          }}
        >
          {part}
        </Box>
      ) : (
        <Fragment key={i}>{part}</Fragment>
      ),
    )}
  </>
);
