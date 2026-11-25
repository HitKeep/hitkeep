import { inject, Service } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { concat, map, Observable, of, switchMap, timer } from 'rxjs';

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
    expected_offer?: string;
    plan_code: 'pro' | 'business';
    billing: BillingInterval;
    locale?: string;
}

export interface AnnualOfferPrice {
    first_invoice_minor: number;
    renewal_minor: number;
    currency: string;
}

export interface CloudBillingState {
    annual_offer?: { code: string; ends_at: string; prices: Record<string, AnnualOfferPrice> };
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
            if (!state.annual_offer) return of(state);
            const remaining = Date.parse(state.annual_offer.ends_at) - Date.now();
            const expired = { ...state, annual_offer: undefined };
            if (!(remaining > 0)) return of(expired);
            return concat(of(state), timer(remaining).pipe(map(() => expired)));
        })
    );
}

@Service()
export class CloudService {
    private readonly http = inject(HttpClient);

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

    getPlans(): Observable<CloudPlanTier[]> {
        return this.http.get<CloudPlanTier[]>('/api/cloud/plans');
    }

    getBillingState(): Observable<CloudBillingState> {
        return this.http.get<CloudBillingState>('/api/cloud/billing/state');
    }
}
