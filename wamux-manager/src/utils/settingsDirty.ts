/** Membership comparison for the event checkboxes. Order does not matter. */
export function sameMembers(a: readonly string[], b: readonly string[]): boolean {
  if (a.length !== b.length) return false;
  const other = new Set(b);
  return a.every((item) => other.has(item));
}

export interface ProxyFields {
  protocol?: string;
  host: string;
  port: string;
  username?: string;
  password?: string;
}

/**
 * The proxy card is dirty when a visible field differs from what is saved.
 * An empty password means "leave the stored one", so it is dirty only when
 * the user has typed a new password.
 */
export function proxyFormDirty(form: ProxyFields, saved: ProxyFields | null): boolean {
  return (
    (form.protocol || "http") !== (saved?.protocol || "http") ||
    form.host !== (saved?.host || "") ||
    form.port !== (saved?.port || "") ||
    (form.username || "") !== (saved?.username || "") ||
    Boolean(form.password)
  );
}
