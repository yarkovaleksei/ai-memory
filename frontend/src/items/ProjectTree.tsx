import ExpandLess from "@mui/icons-material/ExpandLess";
import ExpandMore from "@mui/icons-material/ExpandMore";
import SearchIcon from "@mui/icons-material/Search";
import {
  Box,
  Collapse,
  InputAdornment,
  List,
  ListItemButton,
  ListItemText,
  ListSubheader,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import { useState } from "react";
import { useGetList, useListContext, useTranslate } from "react-admin";
import type { Project } from "../api/bindings";
import { AgentBadge } from "./badges";

// Дерево проектов слева от списка: выбор проекта — это фильтр projectId списка.
// Порядок групп задаёт Go (глобальная память, проекты, нераспознанные, временные);
// временные свёрнуты по умолчанию — это служебные каталоги, а не настоящие проекты.
export const ProjectTree = () => {
  const translate = useTranslate();
  const { filterValues, displayedFilters, setFilters } = useListContext();
  const { data = [], isPending } = useGetList<Project & { id: string }>("projects", {
    pagination: { page: 1, perPage: 10000 },
  });
  const [tempOpen, setTempOpen] = useState(false);
  const [query, setQuery] = useState("");
  const selected = (filterValues.projectId as string | undefined) ?? "";

  const select = (projectId: string) => {
    const next = { ...filterValues };
    if (projectId) next.projectId = projectId;
    else delete next.projectId;
    setFilters(next, displayedFilters);
  };

  // Поиск по имени и пути проекта; выбранный проект виден всегда.
  const q = query.trim().toLowerCase();
  const matches = (p: Project) =>
    !q ||
    p.id === selected ||
    `${p.global ? translate("aim.projects.global") : p.label} ${p.path}`.toLowerCase().includes(q);
  const shown = data.filter(matches);
  const regular = shown.filter((p) => !p.temp && (p.global || p.resolved));
  const unresolved = shown.filter((p) => !p.temp && !p.global && !p.resolved);
  const temp = shown.filter((p) => p.temp);

  const row = (p: Project) => (
    <ProjectRow key={p.id} project={p} selected={p.id === selected} onSelect={() => select(p.id)} />
  );

  return (
    <Box sx={{ display: "flex", flexDirection: "column", height: "100%" }}>
      <Box sx={{ p: 1, borderBottom: 1, borderColor: "divider" }}>
        <TextField
          fullWidth
          placeholder={translate("aim.projects.search")}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          slotProps={{
            htmlInput: { "aria-label": translate("aim.projects.search") },
            input: {
              startAdornment: (
                <InputAdornment position="start">
                  <SearchIcon fontSize="small" />
                </InputAdornment>
              ),
            },
          }}
        />
      </Box>
      <List
        dense
        disablePadding
        sx={{ overflowY: "auto", flex: 1, minHeight: 0 }}
        aria-label={translate("resources.projects.name", { smart_count: 2 })}
      >
        <ListItemButton selected={selected === ""} onClick={() => select("")}>
          <ListItemText primary={translate("aim.projects.all")} />
        </ListItemButton>
        {isPending && (
          <Typography variant="body2" color="text.secondary" sx={{ px: 2, py: 1 }}>
            {translate("aim.projects.loading")}
          </Typography>
        )}
        {!isPending && q && shown.length === 0 && (
          <Typography variant="body2" color="text.secondary" sx={{ px: 2, py: 1 }}>
            {translate("aim.projects.notFound")}
          </Typography>
        )}
        {regular.map(row)}
        {unresolved.length > 0 && (
          <>
            <ListSubheader disableSticky>{translate("aim.projects.unresolved")}</ListSubheader>
            {unresolved.map(row)}
          </>
        )}
        {temp.length > 0 && (
          <>
            <ListItemButton onClick={() => setTempOpen((v) => !v)}>
              <ListItemText
                primary={translate("aim.projects.temp")}
                secondary={translate("aim.projects.count", {
                  smart_count: temp.reduce((n, p) => n + p.itemCount, 0),
                })}
              />
              {tempOpen ? <ExpandLess /> : <ExpandMore />}
            </ListItemButton>
            <Collapse in={tempOpen || !!q || temp.some((p) => p.id === selected)} unmountOnExit>
              <Box sx={{ pl: 1 }}>{temp.map(row)}</Box>
            </Collapse>
          </>
        )}
      </List>
    </Box>
  );
};

interface ProjectRowProps {
  project: Project;
  selected: boolean;
  onSelect: () => void;
}

const ProjectRow = ({ project, selected, onSelect }: ProjectRowProps) => {
  const translate = useTranslate();
  const label = project.global ? translate("aim.projects.global") : project.label;
  const row = (
    <ListItemButton selected={selected} onClick={onSelect}>
      <ListItemText
        primary={label}
        secondary={
          <Stack component="span" spacing={0.5} sx={{ mt: 0.25 }}>
            <Stack direction="row" spacing={0.5} component="span">
              {(project.agents ?? []).map((a) => (
                <AgentBadge key={a} agent={a} />
              ))}
            </Stack>
            <span>{translate("aim.projects.count", { smart_count: project.itemCount })}</span>
          </Stack>
        }
        slotProps={{ primary: { noWrap: true }, secondary: { component: "span" } }}
      />
    </ListItemButton>
  );
  const hint =
    project.path || (project.resolved || project.global ? "" : translate("aim.projects.unresolvedHint"));
  return hint ? (
    <Tooltip title={hint} placement="right">
      {row}
    </Tooltip>
  ) : (
    row
  );
};
