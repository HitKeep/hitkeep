import { nativeAuthReturnPath } from './native-auth';

export function accountDeletionReturnPath(raw: string | null | undefined): string | null {
    return typeof raw === 'string' && /^\/settings\?deletion_attempt=[A-Za-z0-9_-]{43}$/.test(raw) ? raw : null;
}

export function proofAuthReturnPath(raw: string | null | undefined): string | null {
    return accountDeletionReturnPath(raw) ?? nativeAuthReturnPath(raw);
}
