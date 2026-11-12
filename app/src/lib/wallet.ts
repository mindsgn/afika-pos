export function getActiveWalletAddress(wallet: {
  smartAdress?: string | null;
  address?: string | null;
}) {
  return (wallet.smartAdress ?? "").trim().toLowerCase();
}

export function shortenAddress(address?: string | null, start = 6, end = 4) {
  if (!address) return "";
  return `${address.slice(0, start)}...${address.slice(-end)}`;
}
