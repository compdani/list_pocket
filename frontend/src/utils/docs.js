/**
 * Built-in MkDocs manual is served by the app at /docs/ (see cmd/doc_serve.go).
 * Optional override: VITE_DOCS_BASE=/custom/path
 */
const base = (import.meta.env.VITE_DOCS_BASE || '/docs').replace(/\/$/, '');

/**
 * @param {string} path - Path under /docs/, e.g. "templating/#foo" or "maintenance/performance/"
 * @returns {string} Absolute URL path
 */
export function docsUrl(path = '') {
  const p = String(path).replace(/^\/+/, '');
  if (!p) {
    return `${base}/`;
  }
  return `${base}/${p}`;
}
