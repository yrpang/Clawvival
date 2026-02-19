export type DiffItem = {
  key: string;
  before: unknown;
  after: unknown;
};

function isObjectLike(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null;
}

export function buildFlatDiff(
  before: Record<string, unknown> | undefined,
  after: Record<string, unknown> | undefined,
): DiffItem[] {
  const b = before ?? {};
  const a = after ?? {};
  const keys = new Set([...Object.keys(b), ...Object.keys(a)]);
  const out: DiffItem[] = [];

  for (const key of keys) {
    const bv = b[key];
    const av = a[key];

    if (isObjectLike(bv) || isObjectLike(av)) {
      const bo = isObjectLike(bv) ? bv : {};
      const ao = isObjectLike(av) ? av : {};
      const nested = new Set([...Object.keys(bo), ...Object.keys(ao)]);
      for (const nk of nested) {
        const nb = bo[nk];
        const na = ao[nk];
        if (JSON.stringify(nb) !== JSON.stringify(na)) {
          out.push({ key: `${key}.${nk}`, before: nb, after: na });
        }
      }
      continue;
    }

    if (JSON.stringify(bv) !== JSON.stringify(av)) {
      out.push({ key, before: bv, after: av });
    }
  }

  return out;
}
