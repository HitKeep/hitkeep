import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap } from '@angular/router';
import { of } from 'rxjs';

import { AccountDeletionPreparation, UserProfileService } from '@services/user-profile.service';
import { AccountDeletion } from './account-deletion';

const prepared = (): AccountDeletionPreparation => ({
    attempt_id: 'opaque-attempt',
    expires_at: '2099-01-01T00:00:00Z',
    purpose: 'delete-current-account',
    consequence_hash: 'f'.repeat(64),
    phase: 'prepared',
    blockers: [],
    teams: [],
    reauth: { return_path: '/login?returnUrl=%2Fsettings', methods: ['passkey'], verified: false },
    retention: { billing_records: true, cancellation: 'period_end' }
});

describe('AccountDeletion', () => {
    const deleteAccount = vi.fn(() => of(undefined));
    beforeEach(async () => {
        deleteAccount.mockClear();
        await TestBed.configureTestingModule({
            imports: [AccountDeletion],
            providers: [
                { provide: UserProfileService, useValue: { deleteAccount, prepareAccountDeletion: vi.fn(() => of(prepared())) } },
                { provide: ActivatedRoute, useValue: { snapshot: { queryParamMap: convertToParamMap({}) } } }
            ]
        })
            .overrideComponent(AccountDeletion, { set: { template: '', imports: [] } })
            .compileComponents();
    });

    it('requires both fresh proof and explicit final confirmation before issuing DELETE', () => {
        const component = TestBed.createComponent(AccountDeletion).componentInstance;
        const review = prepared();
        component['review'].set(review);
        component['confirmed'].set(true);
        component['deleteAccount']();
        expect(deleteAccount).not.toHaveBeenCalled();
        component['review'].set({ ...review, reauth: { ...review.reauth, verified: true } });
        component['confirmed'].set(false);
        component['deleteAccount']();
        expect(deleteAccount).not.toHaveBeenCalled();
        component['confirmed'].set(true);
        component['deleteAccount']();
        expect(deleteAccount).toHaveBeenCalledOnce();
    });

    it('requires a new server review after changing closure choices', () => {
        const component = TestBed.createComponent(AccountDeletion).componentInstance;
        const review = prepared();
        component['review'].set({ ...review, reauth: { ...review.reauth, verified: true } });
        component['confirmed'].set(true);
        component['selectClosure']('team-id', true);
        component['deleteAccount']();
        expect(deleteAccount).not.toHaveBeenCalled();
        expect(component['confirmed']()).toBe(false);
    });
});
