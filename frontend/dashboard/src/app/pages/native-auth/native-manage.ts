import { DOCUMENT } from '@angular/common';
import { HttpClient, HttpContext } from '@angular/common/http';
import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { MessageModule } from '@openng/optimus-ui/message';
import { firstValueFrom } from 'rxjs';
import { AuthCard } from '@core/components/auth-card/auth-card';
import { Brand } from '@components/brand/brand';
import { browserAppUrl } from '@core/interceptors/base-path.interceptor';
import { SKIP_AUTH_REDIRECT } from '@core/interceptors/auth.interceptor';
import { PasskeyRegistrationStartResponse } from '@services/user-security.service';
import { toPublicKeyCreationOptions, toCreationResponseJson } from '@core/utils/webauthn';

interface ManagementConsent {
    purpose: 'management';
    request: string;
    client_id: string;
    issuer: string;
    email: string;
    csrf_token: string;
    expires_at: string;
    management: {
        action: 'passkey_register' | 'social_link' | 'search_console_connect';
        provider?: string;
        team_id?: string;
        team_name?: string;
        phase: 'pending' | 'authorized' | 'started' | 'mutating' | 'succeeded' | 'failed';
    };
}

@Component({
    selector: 'app-native-manage',
    imports: [ReactiveFormsModule, TranslocoPipe, ButtonModule, InputTextModule, MessageModule, AuthCard, Brand],
    templateUrl: './native-manage.html',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class NativeManagePage {
    private readonly http = inject(HttpClient);
    private readonly document = inject(DOCUMENT);
    private readonly request = inject(ActivatedRoute).snapshot.queryParamMap.get('request') ?? '';
    protected readonly action = browserAppUrl(this.document, '/api/auth/native/authorize');
    protected readonly consent = signal<ManagementConsent | null>(null);
    protected readonly failed = signal(false);
    protected readonly pending = signal(false);
    protected readonly name = new FormControl('', { nonNullable: true });

    constructor() {
        if (!/^[A-Za-z0-9_-]{43}$/.test(this.request)) {
            this.failed.set(true);
            return;
        }
        void this.load();
    }

    private async load(): Promise<void> {
        try {
            const consent = await firstValueFrom(this.http.get<ManagementConsent>('/api/auth/native/consent', { params: { request: this.request } }));
            if (consent.purpose !== 'management' || !consent.management || consent.request !== this.request) throw new Error('invalid consent');
            this.consent.set(consent);
        } catch {
            this.failed.set(true);
        }
    }

    protected submit(event: SubmitEvent): void {
        const decision = (event.submitter as HTMLButtonElement | null)?.value;
        const consent = this.consent();
        if (this.pending() || !consent || Date.parse(consent.expires_at) <= Date.now() || (decision !== 'approve' && decision !== 'deny') || (decision === 'approve' && !['pending', 'succeeded'].includes(consent.management.phase))) {
            event.preventDefault();
            this.failed.set(true);
            return;
        }
        ((event.currentTarget as HTMLFormElement).elements.namedItem('decision') as HTMLInputElement).value = decision;
        this.pending.set(true);
    }

    protected async perform(): Promise<void> {
        const consent = this.consent();
        if (this.pending() || !consent || consent.management.phase !== 'authorized' || Date.parse(consent.expires_at) <= Date.now()) return;
        this.pending.set(true);
        const options = { headers: { 'X-Hitkeep-Native-Management': this.request }, context: new HttpContext().set(SKIP_AUTH_REDIRECT, true) };
        try {
            switch (consent.management.action) {
                case 'passkey_register': {
                    const begin = await firstValueFrom(this.http.post<PasskeyRegistrationStartResponse>('/api/user/security/passkeys/register/start', { name: this.name.value.trim() }, options));
                    const credential = await navigator.credentials.create({ publicKey: toPublicKeyCreationOptions(begin.publicKey) });
                    if (!(credential instanceof PublicKeyCredential)) throw new Error('registration cancelled');
                    const payload = toCreationResponseJson(credential);
                    if (!payload) throw new Error('registration unsupported');
                    await firstValueFrom(this.http.post('/api/user/security/passkeys/register/finish', payload, options));
                    await this.load();
                    break;
                }
                case 'social_link': {
                    const response = await firstValueFrom(this.http.post<{ auth_url: string }>(`/api/user/security/social/${encodeURIComponent(consent.management.provider ?? '')}/start`, {}, options));
                    this.document.defaultView?.location.assign(response.auth_url);
                    break;
                }
                case 'search_console_connect': {
                    const response = await firstValueFrom(this.http.post<{ auth_url: string }>(`/api/user/teams/${encodeURIComponent(consent.management.team_id ?? '')}/integrations/google-search-console/connect`, {}, options));
                    this.document.defaultView?.location.assign(response.auth_url);
                    break;
                }
            }
        } catch {
            this.failed.set(true);
        } finally {
            this.pending.set(false);
        }
    }
}
