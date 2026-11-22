import { inject, provideAppInitializer } from '@angular/core';
import { TranslocoService } from '@jsverse/transloco';
import { firstValueFrom } from 'rxjs';
import { DashboardLanguageService } from './dashboard-language.service';

export function preloadUserLang() {
    const transloco = inject(TranslocoService);
    const language = inject(DashboardLanguageService).activate();
    return firstValueFrom(transloco.load(language), { defaultValue: undefined });
}

export function providePreloadUserLang() {
    return provideAppInitializer(preloadUserLang);
}
