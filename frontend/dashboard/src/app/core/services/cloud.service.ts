import { inject, Service } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { concat, map, Observable, of, ReplaySubject, share, shareReplay, startWith, Subject, switchMap, tap, timer } from 'rxjs';

import { CloudPlanTier } from '@models/analytics.types';

export type CloudPlanCode = 'free' | 'pro' | 'business';
export type BillingInterval = 'monthly' | 'annual';

export interface CloudSignupRequest {
    return_url?: string;
    email: string;
    password: string;
    team_name: string;
    plan_code: CloudPlanCode;
    billing: BillingInterval;
    jurisdiction?: string;
    locale?: string;
    given_name?: string;
    last_name?: string;
    accepted_tos: boolean;
}

export interface CloudSignupResponse {
    status: string;
    plan_code: string;
    billing: BillingInterval;
    retry_after_seconds?: number;
    redirect_url?: string;
    checkout_url?: string;
}

export interface CloudSignupVerificationResendResponse {
    status: 'accepted';
    retry_after_seconds: number;
}

export interface BillingPortalSessionResponse {
    url: string;
}

export interface BillingPortalSessionRequest {
    locale?: string;
}

export interface BillingCheckoutSessionRequest {
    team_id?: string;
    expected_promotion?: string;
    plan_code: 'pro' | 'business';
    billing: BillingInterval;
    locale?: string;
}

export interface PromotionPrice {
    with_code_minor: number;
    regular_minor: number;
    currency: string;
}

export interface CloudBillingState {
    promotion?: { code: string; percent_off: number; ends_at: string; prices: Record<string, Partial<Record<BillingInterval, PromotionPrice>>> };
    team_id: string;
    plan_code: string;
    subscription_status: string;
    pending_plan_code?: 'pro' | 'business';
    billing?: BillingInterval;
}

// Expire visible prices even if the dashboard stays open across the checkout deadline.
export function billingStateUntilExpiry(source: Observable<CloudBillingState>): Observable<CloudBillingState> {
    return source.pipe(
        switchMap((state) => {
            if (!state.promotion) return of(state);
            const remaining = Date.parse(state.promotion.ends_at) - Date.now();
            const expired = { ...state, promotion: undefined };
            if (!(remaining > 0)) return of(expired);
            // Browsers cap a single timeout at a signed 32-bit millisecond delay.
            if (remaining > 2_147_483_647) return concat(of(state), timer(2_147_483_647).pipe(switchMap(() => billingStateUntilExpiry(of(state)))));
            return concat(of(state), timer(remaining).pipe(map(() => expired)));
        })
    );
}

@Service()
export class CloudService {
    private readonly http = inject(HttpClient);

    // Share the regional catalog for the same five-minute window as its origin.
    private readonly plans = this.http.get<CloudPlanTier[]>('/api/cloud/plans').pipe(share({ connector: () => new ReplaySubject<CloudPlanTier[]>(1), resetOnComplete: () => timer(5 * 60_000) }));
    private watched?: { team: string; refresh: Subject<void>; state: Observable<CloudBillingState> };

    watchBillingState(team: string): Observable<CloudBillingState> {
        if (this.watched?.team !== team) {
            const refresh = new Subject<void>();
            const state = refresh.pipe(
                startWith(undefined),
                switchMap(() => billingStateUntilExpiry(this.getBillingState())),
                shareReplay({ bufferSize: 1, refCount: true })
            );
            this.watched = { team, refresh, state };
        }
        return this.watched.state;
    }

    refreshBillingState(team: string): void {
        if (this.watched?.team === team) this.watched.refresh.next();
    }

    signup(payload: CloudSignupRequest): Observable<CloudSignupResponse> {
        return this.http.post<CloudSignupResponse>('/api/cloud/signup', payload);
    }

    resendSignupVerification(email: string): Observable<CloudSignupVerificationResendResponse> {
        return this.http.post<CloudSignupVerificationResendResponse>('/api/cloud/signup/resend-verification', { email });
    }

    createBillingPortalSession(payload: BillingPortalSessionRequest = {}): Observable<BillingPortalSessionResponse> {
        return this.http.post<BillingPortalSessionResponse>('/api/cloud/billing/portal', payload);
    }

    createBillingCheckoutSession(payload: BillingCheckoutSessionRequest): Observable<BillingPortalSessionResponse> {
        return this.http.post<BillingPortalSessionResponse>('/api/cloud/billing/checkout', payload);
    }

    redeemPromotion(team_id: string, code: string): Observable<void> {
        return this.http.post<void>('/api/cloud/billing/promotion', { team_id, code }).pipe(tap(() => this.refreshBillingState(team_id)));
    }

    getPlans(): Observable<CloudPlanTier[]> {
        return this.plans;
    }

    getBillingState({ sync = false } = {}): Observable<CloudBillingState> {
        return this.http.get<CloudBillingState>('/api/cloud/billing/state', { params: sync ? { sync: '1' } : {} });
    }
}
