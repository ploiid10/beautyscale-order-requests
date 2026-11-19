export function formatMoney(minor: number, currency: string): string {
  if (!currency) {
    return (minor / 100).toFixed(2);
  }
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency,
    }).format(minor / 100);
  } catch {
    return `${(minor / 100).toFixed(2)} ${currency}`;
  }
}
