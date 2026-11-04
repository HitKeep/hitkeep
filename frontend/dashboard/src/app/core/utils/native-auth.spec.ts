import { nativeAuthReturnPath, isNativeAuthPage, nativeAuthErrorKey } from './native-auth';

describe('native auth continuation', () => {
    const handle = 'a'.repeat(43);
    const path = `/auth/native/consent?request=${handle}`;
    it('allows only the bounded internal native return', () => {
        expect(nativeAuthReturnPath(path)).toBe(path);
        for (const input of [null, '//evil.test', `${path}&request=other`, `${path}#extra`, `/auth/native/consent?request=${'a'.repeat(42)}`, '/dashboard']) expect(nativeAuthReturnPath(input)).toBeNull();
    });
    it('uses an actionable bounded restart error for expired server continuations', () => {
        expect(nativeAuthErrorKey({ error: { error: 'invalid_grant' } })).toBe('nativeAuth.restart');
        expect(nativeAuthErrorKey({ error: { message: 'private provider error' } })).toBeNull();
        expect(nativeAuthErrorKey(null)).toBeNull();
    });
    it('suppresses signup telemetry on native continuations', () => {
        const page = { defaultView: { location: { pathname: '/prefix/signup', search: `?returnUrl=${encodeURIComponent(path)}` } } } as unknown as Document;
        expect(isNativeAuthPage(page)).toBe(true);
    });
});
