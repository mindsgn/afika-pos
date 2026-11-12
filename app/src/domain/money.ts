/** All money is stored as integer cents (ZAR). */
export function formatZAR(cents: number): string {
  const sign = cents < 0 ? '-' : '';
  const abs = Math.abs(Math.round(cents));
  const rands = Math.floor(abs / 100)
    .toString()
    .replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
  const rem = (abs % 100).toString().padStart(2, '0');
  return `${sign}R${rands}.${rem}`;
}

/** "12", "12.5", "12,50" -> cents. Returns null for invalid input. */
export function parseToCents(input: string): number | null {
  const s = input.trim().replace(',', '.');
  if (!/^\d+(\.\d{1,2})?$/.test(s)) return null;
  const [r, c = ''] = s.split('.');
  return Number(r) * 100 + Number(c.padEnd(2, '0'));
}

export function centsToInput(cents: number): string {
  return (cents / 100).toFixed(2);
}
