import { Box, Paper, Typography } from "@mui/material";
import type { Item } from "../api/bindings";
import { Markdown } from "../items/Markdown";

// Compare — записи рядом: колонка на запись, текст как в просмотре.
export const Compare = ({ items }: { items: Item[] }) => (
  <Box
    sx={{
      display: "grid",
      gridTemplateColumns: `repeat(${Math.min(items.length, 3)}, minmax(0, 1fr))`,
      gap: 1,
    }}
  >
    {items.map((it) => (
      <Paper key={it.id} variant="outlined" sx={{ p: 1.5, overflow: "auto", maxHeight: 360 }}>
        <Typography variant="subtitle2" sx={{ wordBreak: "break-word" }}>
          {it.title}
        </Typography>
        <Markdown body={it.body} links={[]} onOpenItem={() => undefined} />
      </Paper>
    ))}
  </Box>
);
