import { DOCUMENT } from '@angular/common';
import { DestroyRef, inject, signal, Service } from '@angular/core';

interface EmbedCredential {
    token: string;
    siteId: string;
    parentOrigin: string;
}

@Service()
export class EmbedService {
    private readonly view = inject(DOCUMENT).defaultView;
    private readonly active = signal(false);
    private readonly currentCredential = signal<EmbedCredential | null>(null);
    readonly isEmbedMode = this.active.asReadonly();
    readonly credential = this.currentCredential.asReadonly();
    readonly status = signal<'waiting' | 'ready' | 'error'>('waiting');
    private siteId = '';
    private parentOrigin = '';
    private nonce = '';
    private refreshTimer?: ReturnType<typeof setTimeout>;
    private expiryTimer?: ReturnType<typeof setTimeout>;
    private waitingTimer?: ReturnType<typeof setTimeout>;

    constructor() {
        inject(DestroyRef).onDestroy(() => this.stop());
    }

    start(siteId: string, launch: string): boolean {
        this.stop();
        this.active.set(true);
        this.status.set('waiting');
        try {
            // The server verifies this non-secret descriptor before serving the frame.
            const payload = JSON.parse(atob(launch.split('.')[1].replace(/-/g, '+').replace(/_/g, '/'))) as { site_id: string; parent_origin: string; exp: number };
            const origin = new URL(payload.parent_origin);
            if (
                !this.view ||
                this.view.self === this.view.top ||
                payload.site_id !== siteId ||
                !Number.isFinite(payload.exp) ||
                payload.exp * 1000 <= Date.now() ||
                !['https:', 'http:'].includes(origin.protocol) ||
                origin.origin !== payload.parent_origin
            )
                throw new Error('Invalid embed launch');
            this.siteId = siteId;
            this.parentOrigin = origin.origin;
            this.nonce = this.view.crypto.randomUUID();
            this.view.addEventListener('message', this.receive);
            this.requestRefresh();
            return true;
        } catch {
            this.status.set('error');
            return false;
        }
    }

    requestRefresh(): void {
        if (!this.active() || !this.parentOrigin) return;
        this.status.set(this.currentCredential() ? 'ready' : 'waiting');
        if (!this.currentCredential()) {
            clearTimeout(this.waitingTimer);
            this.waitingTimer = setTimeout(() => this.fail(), 20000);
        }
        this.view?.parent.postMessage({ type: 'hitkeep:embed:ready', site_id: this.siteId, nonce: this.nonce }, this.parentOrigin);
    }

    fail(): void {
        clearTimeout(this.refreshTimer);
        clearTimeout(this.expiryTimer);
        clearTimeout(this.waitingTimer);
        this.currentCredential.set(null);
        this.status.set('error');
    }

    stop(): void {
        this.view?.removeEventListener?.('message', this.receive);
        clearTimeout(this.refreshTimer);
        clearTimeout(this.expiryTimer);
        clearTimeout(this.waitingTimer);
        this.currentCredential.set(null);
        this.active.set(false);
        this.siteId = '';
        this.parentOrigin = '';
    }

    private readonly receive = (event: MessageEvent): void => {
        const data = event.data;
        if (!this.active() || event.source !== this.view?.parent || event.origin !== this.parentOrigin || !data || data.site_id !== this.siteId || data.nonce !== this.nonce) return;
        if (data.type === 'hitkeep:embed:error') {
            this.fail();
            return;
        }
        if (data.type !== 'hitkeep:embed:token' || typeof data.token !== 'string' || !data.token.startsWith('hk_embed_') || data.token.length > 8192 || typeof data.expires_at !== 'string') return;
        const remaining = Date.parse(data.expires_at) - Date.now();
        if (!Number.isFinite(remaining) || remaining <= 0 || remaining > 330000) return;
        clearTimeout(this.refreshTimer);
        clearTimeout(this.expiryTimer);
        clearTimeout(this.waitingTimer);
        this.currentCredential.set({ token: data.token, siteId: this.siteId, parentOrigin: this.parentOrigin });
        this.status.set('ready');
        this.refreshTimer = setTimeout(() => this.requestRefresh(), Math.max(1000, remaining - 30000));
        this.expiryTimer = setTimeout(() => this.fail(), remaining);
    };
}
