import { NgOptimizedImage } from '@angular/common';
import { ChangeDetectionStrategy, Component, computed, DestroyRef, inject, input, OnChanges, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { finalize, Subject, takeUntil } from 'rxjs';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { ButtonModule } from '@openng/optimus-ui/button';
import { ConfirmDialogModule } from '@openng/optimus-ui/confirmdialog';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { MessageModule } from '@openng/optimus-ui/message';
import { SelectModule } from '@openng/optimus-ui/select';
import { TagModule } from '@openng/optimus-ui/tag';
import { CopyControl } from '@components/copy-control/copy-control';
import { CrudDialog } from '@components/crud-dialog/crud-dialog';
import { dialogCancelButton, dialogDangerButton } from '@components/dialog-actions/dialog-actions';
import { AppTable, AppTableCell, AppTableColumn, AppTableSlot } from '@components/table/table';
import { TableRowActionItem, TableRowActions } from '@components/table-row-actions/table-row-actions';
import { CountryOption, countryDisplayName, countryOptions } from '@core/i18n/country-options';
import { countryFlagUrl } from '@core/i18n/flag-utils';
import { SettingsCard } from '@features/settings/components/settings-card';
import { IPExclusion } from '@models/analytics.types';
import { CreateExclusionPayload, ExclusionsService } from '@services/exclusions.service';
import { injectActiveLang } from '@core/i18n/active-lang';

const ipOrCIDRPattern = /^(([0-9]{1,3}\.){3}[0-9]{1,3}(\/(3[0-2]|[12]?[0-9]))?|([0-9A-Fa-f:]+)(\/(12[0-8]|1[01][0-9]|[1-9]?[0-9]))?)$/;

interface ActionStatus {
    severity: 'success' | 'error';
    key: string;
    params?: Record<string, string | number>;
}

type ExclusionScope = { kind: 'instance' } | { kind: 'team' | 'site'; id: string; name: string };

type ExclusionRow = IPExclusion & {
    type_label: string;
    scope_label: string;
    value_label: string;
    country_name: string;
    search_value: string;
};

@Component({
    selector: 'app-exclusion-editor',
    imports: [
        ReactiveFormsModule,
        NgOptimizedImage,
        ButtonModule,
        ConfirmDialogModule,
        InputTextModule,
        MessageModule,
        SelectModule,
        TagModule,
        CopyControl,
        CrudDialog,
        AppTable,
        AppTableCell,
        AppTableSlot,
        SettingsCard,
        TableRowActions,
        TranslocoPipe
    ],
    templateUrl: './exclusion-editor.html',
    changeDetection: ChangeDetectionStrategy.OnPush,
    providers: [ConfirmationService]
})
export class ExclusionEditor implements OnChanges {
    readonly scope = input.required<ExclusionScope>();
    private readonly destroyRef = inject(DestroyRef);
    private readonly scopeChanged = new Subject<void>();
    private readonly exclusionsService = inject(ExclusionsService);
    private readonly confirmationService = inject(ConfirmationService);
    private readonly transloco = inject(TranslocoService);
    private readonly activeLanguage = injectActiveLang();

    private readonly requests = computed(() => {
        const scope = this.scope();
        switch (scope.kind) {
            case 'instance':
                return {
                    list: () => this.exclusionsService.listInstanceExclusions(),
                    create: (payload: CreateExclusionPayload) => this.exclusionsService.createInstanceExclusion(payload),
                    remove: (id: string) => this.exclusionsService.deleteInstanceExclusion(id)
                };
            case 'team':
                return {
                    list: () => this.exclusionsService.listTeamExclusions(scope.id, true),
                    create: (payload: CreateExclusionPayload) => this.exclusionsService.createTeamExclusion(scope.id, payload),
                    remove: (id: string) => this.exclusionsService.deleteTeamExclusion(scope.id, id)
                };
            case 'site':
                return {
                    list: () => this.exclusionsService.listSiteExclusions(scope.id, true),
                    create: (payload: CreateExclusionPayload) => this.exclusionsService.createSiteExclusion(scope.id, payload),
                    remove: (id: string) => this.exclusionsService.deleteSiteExclusion(scope.id, id)
                };
        }
    });

    protected readonly prefix = computed(() => ({ instance: 'admin.exclusions', team: 'admin.team.exclusions', site: 'sites.exclusions' })[this.scope().kind]);
    protected readonly tableKey = computed(() => (this.scope().kind === 'instance' ? 'global' : this.scope().kind) + '-exclusions');
    protected readonly contextName = computed(() => {
        const scope = this.scope();
        return scope.kind === 'instance' ? '' : scope.name;
    });
    protected readonly exclusions = signal<IPExclusion[]>([]);
    protected readonly isLoading = signal(false);
    protected readonly isSaving = signal(false);
    protected readonly error = signal<string | null>(null);
    protected readonly createError = signal<string | null>(null);
    protected readonly actionStatus = signal<ActionStatus | null>(null);
    protected readonly deletingRuleID = signal<string | null>(null);
    protected readonly exclusionColumns = computed<AppTableColumn<ExclusionRow>[]>(() => [
        { field: 'value_label', headerKey: `${this.prefix()}.columns.value`, frozen: true },
        { field: 'type_label', headerKey: `${this.prefix()}.columns.type`, type: 'enum', groupable: true },
        { field: 'description', headerKey: `${this.prefix()}.columns.description` },
        { field: 'created_at', headerKey: `${this.prefix()}.columns.created`, type: 'date' }
    ]);
    protected readonly ruleActionLoading = (rule: IPExclusion) => this.deletingRuleID() === rule.id;
    protected readonly isAddDialogVisible = signal(false);
    protected readonly isCurrentIPLoading = signal(false);
    protected readonly currentIPCIDR = signal('');
    protected readonly hasInheritedExclusions = computed(() => this.exclusions().some((rule) => rule.inherited));
    protected readonly ruleTypeOptions = computed(() => {
        this.activeLanguage();
        return (['cidr', 'country', 'user_agent', 'path'] as const).map((value) => ({ label: this.ruleTypeLabel(value), value }));
    });
    protected readonly countryOptions = computed<CountryOption[]>(() => countryOptions(this.activeLanguage()));
    protected readonly exclusionRows = computed<ExclusionRow[]>(() =>
        this.exclusions().map((rule) => {
            const countryName = rule.country_code ? countryDisplayName(rule.country_code, this.activeLanguage()) : '';
            const valueLabel = this.ruleValue(rule, countryName);
            const typeLabel = this.ruleTypeLabel(rule.type);
            const scopeLabel = this.scope().kind === 'instance' ? '' : this.scopeLabel(rule.scope ?? this.scope().kind);
            return {
                ...rule,
                type_label: typeLabel,
                scope_label: scopeLabel,
                value_label: valueLabel,
                country_name: countryName,
                search_value: `${typeLabel} ${scopeLabel} ${valueLabel} ${rule.description ?? ''} ${rule.created_at}`
            };
        })
    );
    protected readonly actionStatusMessage = computed(() => {
        this.activeLanguage();
        const status = this.actionStatus();
        return status ? this.transloco.translate(status.key, status.params) : '';
    });

    protected readonly form = new FormGroup({
        type: new FormControl<IPExclusion['type']>('cidr', { nonNullable: true }),
        cidr: new FormControl('', { nonNullable: true, validators: [Validators.pattern(ipOrCIDRPattern)] }),
        countryCode: new FormControl('', { nonNullable: true }),
        userAgent: new FormControl('', { nonNullable: true }),
        path: new FormControl('', { nonNullable: true }),
        description: new FormControl('', { nonNullable: true, validators: [Validators.maxLength(255)] })
    });

    constructor() {
        this.loadCurrentIP();
    }

    ngOnChanges(): void {
        this.scopeChanged.next();
        this.confirmationService.close();
        this.closeAddDialog();
        this.exclusions.set([]);
        this.reload();
    }

    protected addRule(): void {
        if (this.isSaving() || this.isLoading() || this.deletingRuleID()) {
            return;
        }
        if (!this.validateRuleForm()) {
            this.form.markAllAsTouched();
            return;
        }

        this.createError.set(null);
        this.actionStatus.set(null);
        this.isSaving.set(true);
        const type = this.form.controls.type.value;
        const payload: CreateExclusionPayload = {
            type,
            cidr: type === 'cidr' ? this.form.controls.cidr.value.trim() : undefined,
            country_code: type === 'country' ? this.form.controls.countryCode.value.trim() : undefined,
            user_agent: type === 'user_agent' ? this.form.controls.userAgent.value.trim() : undefined,
            path: type === 'path' ? this.form.controls.path.value.trim() : undefined,
            description: this.form.controls.description.value.trim()
        };
        this.requests()
            .create(payload)
            .pipe(
                takeUntil(this.scopeChanged),
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.isSaving.set(false))
            )
            .subscribe({
                next: (rule) => {
                    this.exclusions.update((current) => {
                        const firstLocalRule = current.findIndex((entry) => !entry.inherited);
                        const insertAt = firstLocalRule < 0 ? current.length : firstLocalRule;
                        return [...current.slice(0, insertAt), rule, ...current.slice(insertAt)];
                    });
                    this.actionStatus.set({ severity: 'success', key: `${this.prefix()}.status.createSuccess`, params: { value: this.ruleValue(rule) } });
                    this.closeAddDialog();
                },
                error: () => this.createError.set(`${this.prefix()}.errors.createFailed`)
            });
    }

    protected openAddDialog(cidr = ''): void {
        this.form.reset({ cidr });
        this.createError.set(null);
        this.isAddDialogVisible.set(true);
    }

    protected onAddDialogVisibleChange(visible: boolean): void {
        if (!visible && !this.isSaving()) {
            this.closeAddDialog();
        }
    }

    protected readonly ruleActions = (rule: IPExclusion): TableRowActionItem[] => {
        if (rule.inherited) {
            return [];
        }
        this.activeLanguage();
        return [
            {
                label: this.transloco.translate('share.dialog.deleteAction'),
                icon: 'pi pi-trash',
                danger: true,
                command: () => this.confirmDeleteRule(rule)
            }
        ];
    };

    protected confirmDeleteRule(rule: IPExclusion): void {
        const scope = this.scope();
        if (this.deletingRuleID() || rule.inherited) {
            return;
        }
        this.confirmationService.confirm({
            header: this.contextName() || this.transloco.translate('admin.exclusions.title'),
            message: this.transloco.translate(`${this.prefix()}.confirmDelete`, { value: this.ruleValue(rule) }),
            rejectButtonProps: dialogCancelButton(this.transloco.translate('common.actions.cancel')),
            acceptButtonProps: dialogDangerButton(this.transloco.translate('share.dialog.deleteAction')),
            accept: () => {
                if (!this.destroyRef.destroyed && scope === this.scope()) this.deleteRule(rule);
            }
        });
    }

    private deleteRule(rule: IPExclusion): void {
        if (rule.inherited || this.deletingRuleID() || this.isLoading() || this.isSaving()) return;

        this.actionStatus.set(null);
        this.deletingRuleID.set(rule.id);
        this.requests()
            .remove(rule.id)
            .pipe(
                takeUntil(this.scopeChanged),
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.deletingRuleID.set(null))
            )
            .subscribe({
                next: () => {
                    this.exclusions.update((current) => current.filter((entry) => entry.id !== rule.id));
                    this.actionStatus.set({ severity: 'success', key: `${this.prefix()}.status.deleteSuccess`, params: { value: this.ruleValue(rule) } });
                },
                error: () => this.actionStatus.set({ severity: 'error', key: `${this.prefix()}.errors.deleteFailed` })
            });
    }

    protected reload(): void {
        if (this.isLoading() || this.isSaving() || this.deletingRuleID()) return;
        this.isLoading.set(true);
        this.error.set(null);
        this.actionStatus.set(null);
        this.requests()
            .list()
            .pipe(
                takeUntil(this.scopeChanged),
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.isLoading.set(false))
            )
            .subscribe({
                next: (rules) => this.exclusions.set(rules),
                error: () => this.error.set(`${this.prefix()}.errors.loadFailed`)
            });
    }

    private loadCurrentIP(): void {
        this.isCurrentIPLoading.set(true);
        this.exclusionsService
            .getCurrentIP()
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.isCurrentIPLoading.set(false))
            )
            .subscribe({
                next: (currentIP) => this.currentIPCIDR.set(currentIP.cidr),
                error: () => this.currentIPCIDR.set('')
            });
    }

    private closeAddDialog(): void {
        this.isAddDialogVisible.set(false);
        this.form.reset();
        this.createError.set(null);
    }

    protected ruleTypeLabel(type: IPExclusion['type']): string {
        this.activeLanguage();
        return this.transloco.translate(`${this.prefix()}.ruleTypes.${type === 'user_agent' ? 'userAgent' : type}`);
    }

    protected scopeLabel(scope: NonNullable<IPExclusion['scope']>): string {
        this.activeLanguage();
        return this.transloco.translate(`${this.prefix()}.scopes.${scope}`);
    }

    protected ruleValue(rule: IPExclusion, countryName?: string): string {
        if (rule.type === 'country' && rule.country_code) {
            return `${countryName ?? countryDisplayName(rule.country_code, this.activeLanguage())} (${rule.country_code})`;
        }
        if (rule.type === 'user_agent') {
            return rule.user_agent ?? '';
        }
        if (rule.type === 'path') {
            return rule.path ?? '';
        }
        return rule.cidr ?? '';
    }

    protected countryFlagUrl(code: string): string {
        return countryFlagUrl(code);
    }

    protected clearValueErrors(): void {
        this.form.controls.cidr.setErrors(null);
        this.form.controls.countryCode.setErrors(null);
        this.form.controls.userAgent.setErrors(null);
        this.form.controls.path.setErrors(null);
    }

    private validateRuleForm(): boolean {
        this.clearValueErrors();
        if (this.form.controls.description.invalid) {
            return false;
        }
        const type = this.form.controls.type.value;
        const controls = this.form.controls;
        if (type === 'country' && !controls.countryCode.value.trim()) {
            controls.countryCode.setErrors({ required: true });
            return false;
        }
        if (type === 'user_agent' && !controls.userAgent.value.trim()) {
            controls.userAgent.setErrors({ required: true });
            return false;
        }
        if (type === 'path' && !controls.path.value.trim()) {
            controls.path.setErrors({ required: true });
            return false;
        }
        if (type === 'cidr' && (!controls.cidr.value.trim() || !ipOrCIDRPattern.test(controls.cidr.value.trim()))) {
            controls.cidr.setErrors({ invalidCidr: true });
            return false;
        }
        return true;
    }
}
