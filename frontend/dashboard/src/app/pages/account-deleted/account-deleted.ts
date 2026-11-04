import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { AuthCard } from '@components/auth-card/auth-card';

@Component({
    selector: 'app-account-deleted',
    imports: [RouterLink, TranslocoPipe, ButtonModule, AuthCard],
    templateUrl: './account-deleted.html',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AccountDeleted {
    protected readonly manualApple = inject(ActivatedRoute).snapshot.queryParamMap.get('manual_revocation') === 'apple';
}
