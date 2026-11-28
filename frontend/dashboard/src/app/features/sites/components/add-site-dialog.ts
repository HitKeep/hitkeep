import { ChangeDetectionStrategy, Component, inject, model, signal } from '@angular/core';

import { Router } from '@angular/router';
import { ButtonModule } from '@openng/optimus-ui/button';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { ReactiveFormsModule, FormControl, Validators } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { TranslocoPipe } from '@jsverse/transloco';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { MessageModule } from '@openng/optimus-ui/message';
import { DialogShell } from '@components/dialog-shell/dialog-shell';
import { SiteService } from '@features/sites/services/site.service';
import { domainValidator, sanitizeDomainInput } from '@features/sites/utils/domain-validator';
@Component({
    selector: 'app-add-site-dialog',
    standalone: true,
    imports: [ReactiveFormsModule, ButtonModule, DialogShell, InputTextModule, MessageModule, TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <app-dialog-shell
            [title]="'sites.addDialog.title' | transloco"
            [visible]="visible()"
            (visibleChange)="onDialogVisibleChange($event)"
            [secondaryLabel]="'common.actions.cancel' | transloco"
            [primaryLabel]="'sites.addDialog.addAction' | transloco"
            [primaryLoading]="isSubmitting()"
            [primaryDisabled]="isSubmitting() || form().invalid()"
            [busy]="isSubmitting()"
            width="450px"
            (primaryAction)="onSubmit()"
        >
            <form (submit)="onSubmit($event)" class="flex flex-col gap-6 pt-2" novalidate>
                <!-- Instructions -->
                <div class="bg-[var(--p-surface-50)] dark:bg-[var(--p-surface-800)] p-3 rounded-md border border-[var(--p-surface-border)] flex gap-3">
                    <i class="pi pi-info-circle text-[var(--p-primary-color)] mt-0.5"></i>
                    <div class="text-sm text-[var(--p-text-muted-color)] leading-relaxed">
                        {{ 'sites.addDialog.instructionsLine1' | transloco: { apex: 'example.com', subdomain: 'blog.example.com' } }}<br />
                        {{ 'sites.addDialog.instructionsLine2' | transloco: { www: 'www' } }}
                    </div>
                </div>

                <div class="flex flex-col gap-2">
                    <label for="domain" class="font-semibold text-sm text-[var(--p-text-color)]">{{ 'sites.addDialog.domainLabel' | transloco }}</label>
                    <input
                        pInputText
                        id="domain"
                        [formControl]="form.domain().control()"
                        [placeholder]="'sites.addDialog.domainPlaceholder' | transloco"
                        class="w-full"
                        (blur)="sanitizeInput()"
                        [class.ng-invalid]="isInvalid()"
                        [class.ng-dirty]="form.domain().dirty()"
                    />

                    <!-- Validation Messages -->
                    @if (isInvalid()) {
                        @if (form.domain().control().hasError('required')) {
                            <small class="text-red-500">{{ 'sites.addDialog.errors.domainRequired' | transloco }}</small>
                        }
                        @if (form.domain().control().hasError('pattern')) {
                            <small class="text-red-500">{{ 'sites.addDialog.errors.domainInvalid' | transloco }}</small>
                        }
                        @if (form.domain().control().hasError('containsProtocol')) {
                            <small class="text-red-500">{{ 'sites.addDialog.errors.removeProtocol' | transloco }}</small>
                        }
                        @if (form.domain().control().hasError('containsWww')) {
                            <small class="text-red-500">{{ 'sites.addDialog.errors.removeWww' | transloco }}</small>
                        }
                    }
                    @if (createError() === 'sites.addDialog.errors.quota' && bootstrap.cloudHosted()) {
                        <p-message severity="warn">
                            {{ 'sites.addDialog.errors.quota' | transloco }}
                            <p-button type="button" variant="text" [label]="'cloud.retentionNotice.upgradeAction' | transloco" (onClick)="openUpgrade()" />
                        </p-message>
                    } @else if (createError()) {
                        <small class="text-red-500">{{ createError() | transloco }}</small>
                    }
                </div>
            </form>
        </app-dialog-shell>
    `
})
export class AddSiteDialog {
    visible = model<boolean>(false);
    protected readonly bootstrap = inject(DashboardBootstrapService);
    private readonly router = inject(Router);
    private siteService = inject(SiteService);
    protected isSubmitting = signal(false);
    protected createError = signal<string | null>(null);
    private readonly formModel = signal({
        domain: new FormControl('', { nonNullable: true, validators: [Validators.required, domainValidator] })
    });
    protected readonly form = compatForm(this.formModel);
    sanitizeInput() {
        this.form.domain().control().setValue(sanitizeDomainInput(this.form.domain().value()));
    }
    protected isInvalid() {
        return this.form.domain().invalid() && (this.form.domain().dirty() || this.form.domain().touched());
    }
    protected openUpgrade(): void {
        this.onDialogVisibleChange(false);
        void this.router.navigateByUrl('/admin/team/overview');
    }

    resetForm() {
        this.form.domain().control().reset('');
        this.createError.set(null);
        this.isSubmitting.set(false);
    }
    protected onDialogVisibleChange(visible: boolean) {
        this.visible.set(visible);
        if (!visible) {
            this.resetForm();
        }
    }
    onSubmit(event?: Event) {
        event?.preventDefault();
        if (this.isSubmitting()) return;
        if (this.form().invalid()) {
            this.form.domain().markAsTouched();
            return;
        }

        const domain = this.form.domain().value();
        if (!domain) {
            this.isSubmitting.set(false);
            return;
        }
        this.isSubmitting.set(true);
        this.createError.set(null);

        const firstSite = this.siteService.sites().length === 0;
        this.siteService.createSite(domain).subscribe({
            next: (site) => {
                if (firstSite) void this.router.navigate(['/sites', site.id, 'settings', 'tracking']);
                this.onDialogVisibleChange(false);
            },
            error: (err) => {
                const message = typeof err.error === 'string' ? err.error : err.error?.message;
                const quota = err.status === 402 || err.error?.code === 'site_limit_reached' || (typeof message === 'string' && message.trim().toLowerCase() === 'site limit reached');
                this.createError.set(quota && this.bootstrap.cloudHosted() ? 'sites.addDialog.errors.quota' : 'sites.addDialog.errors.createFailed');
                this.isSubmitting.set(false);
            }
        });
    }
}
