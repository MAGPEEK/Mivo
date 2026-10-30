// Decimal units as file managers and disk vendors use them. Intl supports these unit names.
const sizeUnits = ["byte", "kilobyte", "megabyte", "gigabyte", "terabyte"] as const;
const unitStep = 1000;
const maximumFractionDigits = 1;

/** Size in bytes for a person, e.g. 276134947 → `276.1 MB` (`en`) or `276,1 MB` (`de`). */
export function formatFileSize(bytes: number, uiLanguage: string): string {
  let value = bytes;
  let unitIndex = 0;
  while (value >= unitStep && unitIndex < sizeUnits.length - 1) {
    value /= unitStep;
    unitIndex += 1;
  }
  return new Intl.NumberFormat(uiLanguage, {
    style: "unit",
    unit: sizeUnits[unitIndex],
    unitDisplay: "short",
    maximumFractionDigits,
  }).format(value);
}
