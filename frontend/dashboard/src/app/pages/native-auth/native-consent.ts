import { DOCUMENT } from '@angular/common';
import { ChangeDetectionStrategy, Component, DestroyRef, inject, signal } from '@angular/core';
import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { CheckboxModule } from '@openng/optimus-ui/checkbox';
import { HttpClient } from '@angular/common/http';
import { ActivatedRoute } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { MessageModule } from '@openng/optimus-ui/message';
import { AuthCard } from '@core/components/auth-card/auth-card';
import { Brand } from '@components/brand/brand';
import { browserAppUrl } from '@core/interceptors/base-path.interceptor';

interface NativeConsent {
    request: string;
    client_id: string;
    issuer: string;
    email: string;
    csrf_token: string;
    remember_me: boolean;
    expires_at: string;
}

@Component({
    selector: 'app-native-consent',
    imports: [ReactiveFormsModule, CheckboxModule, TranslocoPipe, ButtonModule, MessageModule, AuthCard, Brand],
    templateUrl: './native-consent.html',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class NativeConsentPage {
    protected readonly action = browserAppUrl(inject(DOCUMENT), '/api/auth/native/authorize');
    protected readonly remember = new FormControl(false, { nonNullable: true });
    protected readonly consent = signal<NativeConsent | null>(null);
    protected readonly failed = signal(false);
    protected readonly submitting = signal(false);

    constructor() {
        const request = inject(ActivatedRoute).snapshot.queryParamMap.get('request') ?? '';
        if (!/^[A-Za-z0-9_-]{43}$/.test(request)) {
            this.failed.set(true);
            return;
        }
        inject(HttpClient)
            .get<NativeConsent>('/api/auth/native/consent', { params: { request } })
            .pipe(takeUntilDestroyed(inject(DestroyRef)))
            .subscribe({
                next: (consent) => {
                    this.remember.setValue(consent.remember_me);
                    this.consent.set(consent);
                },
                error: () => this.failed.set(true)
            });
    }

    protected submit(event: SubmitEvent): void {
        const decision = (event.submitter as HTMLButtonElement | null)?.value;
        if (this.submitting() || !this.consent() || Date.parse(this.consent()!.expires_at) <= Date.now() || (decision !== 'approve' && decision !== 'deny')) {
            event.preventDefault();
            this.failed.set(true);
            return;
        }
        const field = (event.currentTarget as HTMLFormElement).elements.namedItem('decision') as HTMLInputElement;
        field.value = decision;
        this.submitting.set(true);
    }
}
