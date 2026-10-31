// Colour tokens of the "Console" look (dark), copied from styles/input.css.
export const colors = {
  bg: "#0e0f11",
  panel: "#15171a",
  panel2: "#1c1f23",
  line: "#262a30",
  fg: "#e7e9ec",
  fgMuted: "#9aa1ab",
  fgSubtle: "#6b727c",
  accent: "#3ddc97",
  accentInk: "#04140c",
  accentDim: "rgba(61, 220, 151, 0.14)",
  failed: "#ff5d5d",
  running: "#5aa9ff",
} as const;

export const radius = { sm: 4, md: 6, lg: 10 } as const;
export const font = { xs: 11, sm: 13, base: 14, lg: 16, xl: 20, xxl: 26 } as const;
