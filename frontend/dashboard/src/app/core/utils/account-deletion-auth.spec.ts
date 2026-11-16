import { accountDeletionReturnPath, proofAuthReturnPath } from './account-deletion-auth';

describe('deletion authentication continuation', () => {
    const target = `/settings?deletion_attempt=${'a'.repeat(43)}`;
    it('accepts only the fixed bounded return and keeps native compatibility', () => {
        expect(accountDeletionReturnPath(target)).toBe(target);
        expect(proofAuthReturnPath(target)).toBe(target);
        const native = `/auth/native/consent?request=${'b'.repeat(43)}`;
        expect(proofAuthReturnPath(native)).toBe(native);
        for (const input of [null, '//evil.test', '/settings', `${target}&extra=1`, `${target}#extra`, target.replace('settings', 'other'), target.slice(0, -1)]) expect(accountDeletionReturnPath(input)).toBeNull();
    });
});
