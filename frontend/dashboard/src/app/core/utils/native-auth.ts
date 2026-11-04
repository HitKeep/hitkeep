// Native continuation handles are opaque references; the server owns their bindings.
export function nativeAuthReturnPath(raw: string | null | undefined): string | null {
    return typeof raw === 'string' && /^\/auth\/native\/(?:consent|manage)\?request=[A-Za-z0-9_-]{43}$/.test(raw) ? raw : null;
}

export function isNativeAuthPage(document: Document): boolean {
    const location = document.defaultView?.location;
    if (!location) return false;
    const query = new URLSearchParams(location.search);
    return location.pathname.endsWith('/auth/native/consent') || location.pathname.endsWith('/auth/native/manage') || nativeAuthReturnPath(query.get('returnUrl')) !== null || /^[A-Za-z0-9_-]{43}$/.test(query.get('native') ?? '');
}

export function nativeAuthErrorKey(error: unknown): 'nativeAuth.restart' | null {
    if (!error || typeof error !== 'object' || !('error' in error)) return null;
    const body = error.error;
    return body && typeof body === 'object' && 'error' in body && body.error === 'invalid_grant' ? 'nativeAuth.restart' : null;
}
