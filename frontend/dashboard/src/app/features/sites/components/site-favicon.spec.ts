import { signal } from '@angular/core';
import { EmbedService } from '@services/embed.service';
import { ComponentFixture, TestBed } from '@angular/core/testing';

import { SiteFavicon, SiteFaviconSource } from './site-favicon';

describe('SiteFavicon', () => {
    let fixture: ComponentFixture<SiteFavicon>;
    const embedded = signal(false);

    beforeEach(async () => {
        embedded.set(false);
        await TestBed.configureTestingModule({
            imports: [SiteFavicon],
            providers: [{ provide: EmbedService, useValue: { isEmbedMode: embedded } }]
        }).compileComponents();

        fixture = TestBed.createComponent(SiteFavicon);
    });

    it('falls back to a tokenized icon when the favicon cannot load', () => {
        fixture.componentRef.setInput('site', site('example.com'));
        fixture.detectChanges();

        const image = fixture.nativeElement.querySelector('img') as HTMLImageElement;
        image.dispatchEvent(new Event('error'));
        fixture.detectChanges();

        expect(fixture.nativeElement.querySelector('img')).toBeNull();
        expect(fixture.nativeElement.querySelector('.hk-site-favicon-fallback')).toBeTruthy();
    });

    it('uses the local fallback in an embed without issuing a cookie-bearing favicon image request', () => {
        embedded.set(true);
        fixture.componentRef.setInput('site', site('example.com'));
        fixture.detectChanges();

        expect(fixture.nativeElement.querySelector('img')).toBeNull();
        expect(fixture.nativeElement.querySelector('.hk-site-favicon-fallback')).toBeTruthy();

        embedded.set(false);
        fixture.detectChanges();
        expect(fixture.nativeElement.querySelector('img')).toBeTruthy();
    });

    it('marks the favicon image as decorative when adjacent text names the site', () => {
        fixture.componentRef.setInput('site', site('example.com'));
        fixture.detectChanges();

        const image = fixture.nativeElement.querySelector('img') as HTMLImageElement;

        expect(image.getAttribute('alt')).toBe('');
        expect(image.getAttribute('aria-hidden')).toBe('true');
    });

    it('tries the image again when the site changes after a failed favicon', () => {
        fixture.componentRef.setInput('site', site('example.com'));
        fixture.detectChanges();
        (fixture.nativeElement.querySelector('img') as HTMLImageElement).dispatchEvent(new Event('error'));
        fixture.detectChanges();

        fixture.componentRef.setInput('site', site('next.example.com'));
        fixture.detectChanges();

        expect(fixture.nativeElement.querySelector('img')).toBeTruthy();
        expect(fixture.nativeElement.querySelector('.hk-site-favicon-fallback')).toBeNull();
    });
});

function site(domain: string): SiteFaviconSource {
    return { domain };
}
