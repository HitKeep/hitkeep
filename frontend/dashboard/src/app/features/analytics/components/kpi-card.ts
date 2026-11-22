import { ChangeDetectionStrategy, Component, computed, input, OnChanges, OnDestroy, output, signal, SimpleChanges } from '@angular/core';
import { AnimatedDuration } from '@components/animated-duration/animated-duration';
import { AnimatedNumber } from '@components/animated-number/animated-number';
import { CardModule } from '@openng/optimus-ui/card';
import { SkeletonModule } from '@openng/optimus-ui/skeleton';
import { injectSkeletonGate } from '@services/report-subject.service';
import { KPI_DELTA_FORMAT, kpiDeltaClass } from './kpi-comparison-metric';
import { KpiComparisonTrigger } from './kpi-comparison-trigger';

export { KPI_DELTA_FORMAT } from './kpi-comparison-metric';

export const KPI_PERCENT_FORMAT: Intl.NumberFormatOptions = Object.freeze({
    minimumFractionDigits: 1,
    maximumFractionDigits: 1
});

export const KPI_ONE_DECIMAL_FORMAT: Intl.NumberFormatOptions = Object.freeze({
    minimumFractionDigits: 1,
    maximumFractionDigits: 1
});

export const KPI_SHORT_DECIMAL_FORMAT: Intl.NumberFormatOptions = Object.freeze({
    minimumFractionDigits: 1,
    maximumFractionDigits: 2
});

export const KPI_MONEY_FALLBACK_FORMAT: Intl.NumberFormatOptions = Object.freeze({
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
});

export interface KpiCardModel {
    label: string;
    value: string | number;
    loading: boolean;
    valueClass?: string;
    format?: Intl.NumberFormatOptions;
    prefix?: string;
    suffix?: string;
    duration?: boolean;
    invertDelta?: boolean;
    updateKey?: number;
}

@Component({
    selector: 'app-kpi-card',
    standalone: true,
    imports: [CardModule, SkeletonModule, AnimatedDuration, AnimatedNumber, KpiComparisonTrigger],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <p-card [class]="flat() ? 'hk-kpi-flat' : 'shadow-sm h-full border border-surface-200 dark:border-surface-700 surface-card'">
            <div class="hk-kpi-card__body flex flex-col gap-2">
                @for (cue of updateCues(); track cue) {
                    <span class="hk-kpi-card__update-cue" [attr.data-update-cue]="cue" aria-hidden="true"></span>
                }
                <span class="hk-kpi-card__content text-sm font-medium text-muted-color">{{ label() }}</span>
                <div [class]="displayClass()">
                    @if (showSkeleton()) {
                        <p-skeleton width="60%" height="2rem" />
                    } @else {
                        @let numeric = numericValue();
                        @if (numeric !== null) {
                            @if (duration()) {
                                <app-animated-duration [value]="numeric" />
                            } @else {
                                <app-animated-number [value]="numeric" [format]="format()" [prefix]="prefix()" [suffix]="suffix()" />
                            }
                        } @else {
                            {{ value() }}
                        }
                    }
                </div>
                @let deltaValue = normalizedDelta();
                @if (!showSkeleton() && comparable() && numericValue() !== null) {
                    <app-kpi-comparison-trigger class="hk-kpi-card__content self-start" [metricLabel]="label()" [delta]="delta()" [invertDelta]="invertDelta()" (compare)="compare.emit($event)" />
                } @else if (!showSkeleton() && deltaValue !== null) {
                    <div class="hk-kpi-card__content flex items-center gap-1">
                        <span [class]="deltaClass()"><app-animated-number [value]="deltaValue" [format]="deltaFormat" /></span>
                    </div>
                }
                @if (!showSkeleton() && description()) {
                    <span class="hk-kpi-card__content text-xs text-muted-color">{{ description() }}</span>
                }
            </div>
        </p-card>
    `,
    styles: [
        `
            .hk-kpi-flat {
                --p-card-shadow: none;
                --p-card-background: transparent;
                --p-card-body-padding: 0.625rem 0.85rem;
            }

            .hk-kpi-flat .hk-kpi-card__body {
                display: grid;
                grid-template-columns: auto 1fr;
                align-items: center;
                gap: 0 0.25rem;
            }
            .hk-kpi-flat .hk-kpi-card__body > span:not(.hk-kpi-card__update-cue) {
                grid-column: 1 / -1;
            }
            .hk-kpi-flat .hk-kpi-card__body > div {
                font-size: clamp(1.25rem, 2vw, 1.75rem);
            }
            .hk-kpi-flat app-kpi-comparison-trigger {
                justify-self: start;
            }
            @media (min-width: 48rem) {
                .hk-kpi-flat .hk-kpi-card__body {
                    display: flex;
                    align-items: flex-start;
                    gap: 0;
                }
            }
            .hk-kpi-card__body {
                border-radius: 0.5rem;
                isolation: isolate;
                margin: -0.25rem;
                overflow: hidden;
                padding: 0.25rem;
                position: relative;
            }

            .hk-kpi-card__content {
                position: relative;
                z-index: 1;
            }

            .hk-kpi-card__update-cue {
                background: color-mix(in srgb, var(--p-text-color) 5%, transparent);
                border-radius: inherit;
                display: none;
                inset: 0;
                pointer-events: none;
                position: absolute;
                z-index: 0;
            }

            @media (prefers-reduced-motion: reduce) {
                .hk-kpi-card__update-cue {
                    display: block;
                }
            }
        `
    ]
})
export class KpiCard implements OnChanges, OnDestroy {
    label = input.required<string>();
    value = input.required<string | number>();
    loading = input<boolean>(false);
    updateKey = input<number>(0);
    valueClass = input<string>('');
    format = input<Intl.NumberFormatOptions>();
    prefix = input<string>();
    suffix = input<string>();
    duration = input<boolean>(false);
    delta = input<number | null>(null);
    invertDelta = input<boolean>(false);
    description = input('');
    comparable = input(false);
    flat = input(false);
    compare = output<HTMLButtonElement>();

    private cueSequence = 0;
    private cueTimer: ReturnType<typeof setTimeout> | null = null;
    /** Whether a value was on screen before the change currently being handled. */
    private hasPaintedValue = false;

    protected readonly showSkeleton = injectSkeletonGate(this.loading);
    protected readonly updateCues = signal<readonly number[]>([]);
    protected readonly deltaFormat = KPI_DELTA_FORMAT;
    protected displayClass = computed(() => `hk-kpi-card__content ${this.valueClass() || 'text-2xl xl:text-3xl font-bold'}`);
    protected numericValue = computed(() => {
        const value = this.value();
        return typeof value === 'number' && Number.isFinite(value) ? value : null;
    });
    protected normalizedDelta = computed(() => {
        const delta = this.delta();
        if (delta === null || !Number.isFinite(delta)) return null;
        return delta / 100;
    });
    protected readonly deltaClass = computed(() => kpiDeltaClass(this.delta(), this.invertDelta()));

    ngOnChanges(changes: SimpleChanges): void {
        const skeleton = this.showSkeleton();
        const wasPainted = this.hasPaintedValue;
        this.hasPaintedValue = !skeleton;
        if (skeleton || !wasPainted) {
            // Nothing was on screen to change: the first value after a skeleton
            // is an arrival, not an update.
            return;
        }

        const value = changes['value'];
        const valueChanged = !!value && !value.firstChange && !Object.is(value.previousValue, value.currentValue);
        // An advancing key cues a refresh that happened to land on the same
        // number, which a value comparison alone cannot see.
        const updateKey = changes['updateKey'];
        const keyAdvanced = !!updateKey && !updateKey.firstChange && Number(updateKey.currentValue) > Number(updateKey.previousValue);
        if (!valueChanged && !keyAdvanced) {
            return;
        }

        this.showUpdateCue();
    }

    ngOnDestroy(): void {
        if (this.cueTimer) {
            clearTimeout(this.cueTimer);
        }
    }

    private showUpdateCue(): void {
        if (this.cueTimer) {
            clearTimeout(this.cueTimer);
        }

        this.updateCues.set([++this.cueSequence]);
        this.cueTimer = setTimeout(() => {
            this.updateCues.set([]);
            this.cueTimer = null;
        }, 600);
    }
}
