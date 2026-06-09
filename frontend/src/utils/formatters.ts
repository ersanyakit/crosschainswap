export function formatPrice(value: number, minimumFractionDigits = 2, maximumFractionDigits?: number): string {
  if (!Number.isFinite(value)) return '0.00';
  const abs = Math.abs(value);
  const maxFractionDigits = maximumFractionDigits ?? 2;
  if (abs > 0 && abs < 0.01) {
    return value.toLocaleString(undefined, {
      minimumFractionDigits: maximumFractionDigits === undefined ? 0 : minimumFractionDigits,
      maximumFractionDigits: maximumFractionDigits ?? 12,
    });
  }
  return value.toLocaleString(undefined, {
    minimumFractionDigits,
    maximumFractionDigits: maxFractionDigits,
  });
}

export function formatQuantity(value: number, fractionDigits = 4): string {
  if (!Number.isFinite(value)) return '0';
  if (Math.abs(value) >= 1000) {
    return value.toLocaleString(undefined, { maximumFractionDigits: fractionDigits });
  }
  return value.toLocaleString(undefined, {
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  });
}
