import type { IWelcomeAction, IWelcomeLegalLink, IWelcomeToken } from "./types";

export const DEFAULT_WORDMARK = "afika";

export const DEFAULT_HEADLINE: IWelcomeToken[] = [
  { kind: "word", text: "Your" },
  { kind: "word", text: "business," },
  { kind: "word", text: "ready", muted: true },
  { kind: "word", text: "to" },
  { kind: "word", text: "sell." },
  { kind: "word", text: "Simple." },
];

export const DEFAULT_ACTIONS: IWelcomeAction[] = [
  {
    key: "google",
    label: "Sign in with Google",
    variant: "primary",
  },
  {
    key: "apple",
    label: "Sign in with Apple",
    variant: "primary",
  },
  {
    key: "login",
    label: "I have an account",
    variant: "secondary",
  },
];

export const DEFAULT_LEGAL_PREFIX = "Secure sign in for Afika POS";
export const DEFAULT_LEGAL_LINKS: IWelcomeLegalLink[] = [
  { key: "terms", label: "Built for local business" },
];
export const DEFAULT_LEGAL_SUFFIX = "";

export const FONTS = {
  bold: "Inter_700Bold",
  semiBold: "Inter_600SemiBold",
  medium: "Inter_500Medium",
  regular: "Inter_400Regular",
} as const;

export const HEADLINE_SIZE = 55;
export const HEADLINE_LINE_HEIGHT = 54;
export const AVATAR_SIZE = 50;
export const ICON_SIZE = 17;
export const CONTENT_HORIZONTAL_PADDING = 23;
export const ACTION_HEIGHT = 54;
export const ACTION_RADIUS = 14;
export const ACTION_GAP = 15;
export const WORDMARK_SIZE = 28;

export const COLORS = {
  screen: "#F4F5F7",
  wordmark: "#1C1C1E",
  headline: "#1C1C1E",
  headlineMuted: "#9A9CA3",
  primary: "#1C1C1E",
  primaryPressed: "#2A2A2D",
  primaryLabel: "#D2E95F",
  secondary: "#f0f0f0",
  secondaryPressed: "#e4e4e4",
  secondaryLabel: "#111111",
  legal: "#9a9a9a",
  legalLink: "#818181",
} as const;
