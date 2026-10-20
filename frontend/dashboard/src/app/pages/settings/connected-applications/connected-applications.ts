import { ChangeDetectionStrategy, Component, DestroyRef, computed, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { MessageModule } from '@openng/optimus-ui/message';
import { finalize } from 'rxjs';

import { OAuthGrant } from '@models/oauth.types';
import { OAuthService } from '@services/oauth.service';
import { formatOAuthTime, oauthScopeLabelKey } from '@core/utils/oauth-display';
import { injectActiveLang } from '@core/i18n/active-lang';
import { PageFrame } from '@components/page-frame/page-frame';
import { PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';

@Component({
    selector: 'app-connected-applications',
    imports: [TranslocoPipe, ButtonModule, MessageModule, PageFrame],
    templateUrl: './connected-applications.html',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ConnectedApplicationsPage {
    protected readonly scopeLabelKey = oauthScopeLabelKey;
    private readonly oauth = inject(OAuthService);
    private readonly destroyRef = inject(DestroyRef);
    private readonly transloco = inject(TranslocoService);
    private readonly language = injectActiveLang();
    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.language();
        return [
            { label: this.transloco.translate('userMenu.userSettings'), routerLink: '/settings' },
            { label: this.transloco.translate('oauth.applicationsTitle'), isCurrent: true }
        ];
    });
    protected readonly grants = signal<OAuthGrant[]>([]);
    protected readonly loading = signal(false);
    protected readonly loadError = signal<string | null>(null);
    protected readonly pending = signal(new Set<string>());
    protected readonly failures = signal(new Set<string>());
    protected readonly revoked = signal(false);

    constructor() {
        this.load();
    }

    protected load(): void {
        if (this.loading()) return;
        this.loading.set(true);
        this.loadError.set(null);
        this.oauth
            .grants()
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.loading.set(false))
            )
            .subscribe({
                next: (grants) => this.grants.set(grants),
                error: (error: { status?: number }) => this.loadError.set(error.status === 401 || error.status === 403 ? 'oauth.permissionDenied' : 'oauth.loadFailed')
            });
    }

    protected revoke(grant: OAuthGrant): void {
        if (this.pending().has(grant.id)) return;
        this.pending.update((ids) => new Set(ids).add(grant.id));
        this.failures.update((ids) => {
            const next = new Set(ids);
            next.delete(grant.id);
            return next;
        });
        this.revoked.set(false);
        this.oauth
            .revoke(grant.id)
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() =>
                    this.pending.update((ids) => {
                        const next = new Set(ids);
                        next.delete(grant.id);
                        return next;
                    })
                )
            )
            .subscribe({
                next: () => {
                    this.grants.update((grants) => grants.filter((item) => item.id !== grant.id));
                    this.revoked.set(true);
                },
                error: () => this.failures.update((ids) => new Set(ids).add(grant.id))
            });
    }

    protected formatTime(value: string | null | undefined): string {
        const language = this.language();
        if (!value) return this.transloco.translate('oauth.neverUsed');
        return formatOAuthTime(value, language) ?? this.transloco.translate('oauth.unavailable');
    }
}
