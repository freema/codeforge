export interface MetricEntry {
  name: string;
  labels: Record<string, string>;
  value: number;
}

export function parsePrometheusMetrics(text: string): MetricEntry[] {
  const entries: MetricEntry[] = [];

  for (const line of text.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;

    const match = trimmed.match(
      /^([a-zA-Z_:][a-zA-Z0-9_:]*)(?:\{(.+?)\})?\s+(.+)$/,
    );
    if (!match) continue;

    const [, name, labelsStr, valueStr] = match;
    if (!name || !valueStr) continue;

    const labels: Record<string, string> = {};
    if (labelsStr) {
      for (const pair of labelsStr.match(/[a-zA-Z_]+="[^"]*"/g) ?? []) {
        const eqIdx = pair.indexOf("=");
        const key = pair.slice(0, eqIdx);
        const val = pair.slice(eqIdx + 2, -1); // strip quotes
        if (key) labels[key] = val ?? "";
      }
    }

    entries.push({ name, labels, value: parseFloat(valueStr) });
  }

  return entries;
}

export function getMetricValue(
  entries: MetricEntry[],
  name: string,
  labels?: Record<string, string>,
): number {
  const match = entries.find(
    (e) =>
      e.name === name &&
      (!labels || Object.entries(labels).every(([k, v]) => e.labels[k] === v)),
  );
  return match?.value ?? 0;
}

export function sumMetricValues(entries: MetricEntry[], name: string): number {
  return entries
    .filter((e) => e.name === name)
    .reduce((sum, e) => sum + e.value, 0);
}
