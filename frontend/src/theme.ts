import "@fontsource/roboto/latin-400.css";
import "@fontsource/roboto/latin-500.css";
import "@fontsource/roboto/latin-700.css";
import "@fontsource/roboto/latin-ext-400.css";
import "@fontsource/roboto/latin-ext-500.css";
import "@fontsource/roboto/latin-ext-700.css";
import "@fontsource/roboto/cyrillic-400.css";
import "@fontsource/roboto/cyrillic-500.css";
import "@fontsource/roboto/cyrillic-700.css";
import "@fontsource/roboto/cyrillic-ext-400.css";
import "@fontsource/roboto/cyrillic-ext-500.css";
import "@fontsource/roboto/cyrillic-ext-700.css";
import { alpha, darken, type PaletteOptions, type Theme } from "@mui/material";
import { defaultDarkTheme, defaultLightTheme, type RaThemeOptions } from "react-admin";
import * as colors from "./colors";

// Тема поверх штатной react-admin.
//
// Шрифт Roboto встроен в сборку (ADR-016): MUI рассчитан на его метрики, а в
// системе его может не быть — WebKitGTK тогда подставляет, например, Noto Sans,
// и текст в кнопках съезжает по вертикали. Подключены только латиница и
// кириллица: остальные алфавиты в памяти редки, их возьмёт системный шрифт.
//
// Размеры элементов управления задаются только здесь, компоненты их не
// переопределяют: поле, кнопка, кнопка-переключатель и кнопка-иконка — одной
// высоты CONTROL, поля — одного вида (outlined, small), переключатели — одного
// размера. Так строка фильтров и панель действий выглядят ровно.
const CONTROL = 40;

// margin: "none" — отступы между элементами задают Stack и формы; с margin
// у поля, но не у кнопки рядом они стояли бы на разной высоте.
const field = { variant: "outlined", size: "small", margin: "none" } as const;

// Палитра: роли состояний общие, primary у светлой темы малиновый, у тёмной —
// синий (малиновый на тёмном фоне читается плохо).
//
// На жёлтом и сером белый текст не читается (контраст ниже 3:1) — у них тёмный.
// dark у warning и info темнее штатного: им окрашен текст контурных чипов на
// белом фоне, и он должен держать контраст 4.5:1 для мелкого текста.
const role = (main: string, contrastText: string = colors.CONTRAST_TEXT) => ({ main, contrastText });
const basePalette: PaletteOptions = {
  secondary: role(colors.SECONDARY, colors.DARK_TEXT),
  error: role(colors.ERROR),
  warning: { ...role(colors.WARNING, colors.DARK_TEXT), dark: darken(colors.WARNING, 0.45) },
  info: { ...role(colors.INFO), dark: darken(colors.INFO, 0.2) },
  success: role(colors.SUCCESS),
};

const withAim = (base: RaThemeOptions, palette: PaletteOptions): RaThemeOptions => ({
  ...base,
  palette: { ...base.palette, ...basePalette, ...palette },
  typography: { ...base.typography, fontFamily: '"Roboto", "Helvetica", "Arial", sans-serif' },
  components: {
    ...base.components,
    MuiTextField: { ...base.components?.MuiTextField, defaultProps: { ...field, fullWidth: true } },
    MuiFormControl: {
      ...base.components?.MuiFormControl,
      defaultProps: { ...field, fullWidth: true },
      // Поля react-admin сами ставят margin="dense"; без отступов они стоят
      // вровень с кнопками и полями без него.
      styleOverrides: { marginDense: { marginTop: 0, marginBottom: 0 } },
    },
    // Подписи кнопок — как написаны в словаре, без капса: так читаются русские фразы.
    MuiButton: {
      defaultProps: { size: "medium" },
      styleOverrides: { root: { textTransform: "none", lineHeight: 1.5, minHeight: CONTROL } },
    },
    MuiToggleButton: {
      defaultProps: { size: "medium" },
      styleOverrides: { root: { textTransform: "none", lineHeight: 1.5, height: CONTROL } },
    },
    MuiIconButton: {
      defaultProps: { size: "medium" },
      styleOverrides: { root: { width: CONTROL, height: CONTROL } },
    },
    // Переключатель и отметка — штатного размера везде, в том числе в фильтрах:
    // маленькие из-за size="small" у поля рядом читались плохо.
    MuiSwitch: { defaultProps: { size: "medium" } },
    // Подпись с переключателем — той же высоты, что поле: в строке фильтров
    // они стоят на одной линии.
    MuiFormControlLabel: { styleOverrides: { root: { minHeight: CONTROL } } },
    MuiCheckbox: { defaultProps: { size: "medium" } },
    // Все маленькие чипы одной высоты: значки агента, типа, вида записи и
    // режима правки стоят в одной строке.
    MuiChip: {
      styleOverrides: {
        sizeSmall: { height: 20, fontSize: "0.75rem" },
        labelSmall: { paddingLeft: 7, paddingRight: 7 },
        // На белом фоне контурный чип окрашен тёмным оттенком цвета: жёлтый
        // и голубой текст на белом не читаются. В тёмной теме — штатный main.
        outlinedWarning: ({ theme }: { theme: Theme }) =>
          theme.palette.mode === "light"
            ? { color: theme.palette.warning.dark, borderColor: theme.palette.warning.dark }
            : {},
        outlinedInfo: ({ theme }: { theme: Theme }) =>
          theme.palette.mode === "light"
            ? { color: theme.palette.info.dark, borderColor: theme.palette.info.dark }
            : {},
      },
    },
    // react-admin всегда рисует верхнюю панель с color="secondary", а secondary
    // у нас серый: в светлой теме панель — основного цвета. В тёмной MUI
    // и так красит её в цвет фона.
    MuiAppBar: {
      styleOverrides: {
        colorSecondary: ({ theme }: { theme: Theme }) =>
          theme.palette.mode === "light"
            ? { backgroundColor: theme.palette.primary.main, color: theme.palette.primary.contrastText }
            : {},
      },
    },
    // Выпадающие списки не раскрываются на весь экран: длинные — прокручиваются.
    MuiMenu: {
      styleOverrides: { paper: { maxHeight: 360 } },
    },
    // Активный пункт бокового меню заметен: цвет, фон и полоса слева.
    RaMenuItemLink: {
      styleOverrides: {
        root: ({ theme }: { theme: Theme }) => ({
          borderLeft: "3px solid transparent",
          "&.RaMenuItemLink-active": {
            borderLeftColor: theme.palette.primary.main,
            backgroundColor: alpha(theme.palette.primary.main, 0.12),
            color: theme.palette.primary.main,
            fontWeight: 500,
            "& .RaMenuItemLink-icon": { color: theme.palette.primary.main },
          },
        }),
      },
    },
  },
});

export const lightTheme = withAim(defaultLightTheme, { mode: "light", primary: role(colors.PRIMARY) });
export const darkTheme = withAim(defaultDarkTheme, { mode: "dark", primary: role(colors.BLUE) });
