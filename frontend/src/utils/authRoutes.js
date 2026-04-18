export function toAdminPath(next = '/') {
  if (typeof next !== 'string' || !next.trim()) {
    return '/admin/';
  }

  if (next.startsWith('/admin')) {
    return next;
  }

  if (next === '/') {
    return '/admin/';
  }

  return `/admin${next.startsWith('/') ? next : `/${next}`}`;
}

export function toRouterPath(next = '/') {
  if (typeof next !== 'string' || !next.trim()) {
    return '/';
  }

  if (next === '/admin' || next === '/admin/') {
    return '/';
  }

  if (next.startsWith('/admin/')) {
    return next.slice('/admin'.length);
  }

  return next;
}

export function getRouteNext(route) {
  return typeof route.query.next === 'string' && route.query.next ? route.query.next : '/';
}
