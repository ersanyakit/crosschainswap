export function formatPrice(value: number, minimumFractionDigits = 2): string {
  if (!Number.isFinite(value)) return '0.00';
  const abs = Math.abs(value);
  if (abs > 0 && abs < 0.01) {
    return value.toLocaleString(undefined, {
      minimumFractionDigits: 0,
      maximumFractionDigits: 12,
    });
  }
  return value.toLocaleString(undefined, {
    minimumFractionDigits,
    maximumFractionDigits: 2,
  });
}

export function formatQuantity(value: number): string {
  if (!Number.isFinite(value)) return '0';
  if (Math.abs(value) >= 1000) {
    return value.toLocaleString(undefined, { maximumFractionDigits: 0 });
  }
  return value.toLocaleString(undefined, { minimumFractionDigits: 4, maximumFractionDigits: 4 });
}
