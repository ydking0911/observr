// sdk/node/src/traceparent.ts
const LOWER_HEX = /^[0-9a-f]+$/;

/**
 * Parse a W3C traceparent header.
 * Returns { traceId, parentId } on success, null for any invalid input.
 * parentId should be used as parent_span_id so the causal chain links correctly.
 */
export function parseTraceparent(
  header: string
): { traceId: string; parentId: string } | null {
  const parts = header.split("-");
  if (parts.length !== 4 || parts[0] !== "00") return null;
  const [, traceId, parentId, flags] = parts;
  if (traceId.length !== 32 || parentId.length !== 16 || flags.length !== 2)
    return null;
  if (
    !LOWER_HEX.test(traceId) ||
    !LOWER_HEX.test(parentId) ||
    !LOWER_HEX.test(flags)
  )
    return null;
  if (/^0+$/.test(traceId) || /^0+$/.test(parentId)) return null;
  return { traceId, parentId };
}

/** Format a W3C traceparent header value. */
export function formatTraceparent(traceId: string, spanId: string): string {
  return `00-${traceId}-${spanId}-01`;
}
