import { fonts } from "./fonts";

export const typography = {
  title: {
    fontFamily: fonts.semibold,
    fontSize: 28,
    fontWeight: "600" as const,
  },
  button: {
    fontFamily: fonts.medium,
    fontSize: 16,
    fontWeight: "500" as const,
  },
  balance: {
    fontFamily: fonts.medium,
    fontSize: 48,
    fontWeight: "500" as const,
  },
  subtitle: {
    fontFamily: fonts.medium,
    fontSize: 18,
    fontWeight: "500" as const,
  },
  body: {
    fontFamily: fonts.regular,
    fontSize: 14,
    fontWeight: "400" as const,
  },
  caption: {
    fontFamily: fonts.regular,
    fontSize: 12,
    fontWeight: "400" as const,
  },
};
