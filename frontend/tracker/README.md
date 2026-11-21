# @hitkeep/tracker

[![npm version](https://img.shields.io/npm/v/@hitkeep/tracker.svg)](https://www.npmjs.com/package/@hitkeep/tracker)
[![license](https://img.shields.io/npm/l/@hitkeep/tracker.svg)](https://github.com/HitKeep/hitkeep/blob/main/LICENSE)
[![zero dependencies](https://img.shields.io/badge/dependencies-0-brightgreen.svg)](https://www.npmjs.com/package/@hitkeep/tracker?activeTab=dependencies)
[![types included](https://img.shields.io/badge/types-included-blue.svg)](https://www.npmjs.com/package/@hitkeep/tracker)

Typed, cookieless web analytics tracking for **React, Vue, Angular, Astro**, and any other bundler-based frontend — a privacy-friendly Google Analytics alternative that stores no cookies and needs no consent banner for basic measurement.

This is the official browser SDK for [HitKeep](https://hitkeep.com), a self-hostable, GDPR-friendly analytics platform that runs as a single Go binary. The package bundles the same tracker that ships as HitKeep's `hk.js` snippet — compiled from the same source — as a fully typed ES module.

- **Cookieless by default** — no cookies, no cross-site identifiers, no consent banner for basic analytics
- **Framework-native** — no `<script>` tag, no `window.hk` casts, full TypeScript types
- **Automatic tracking** — pageviews, SPA route changes, outbound links, file downloads, form submits
- **Typed e-commerce events** — `view_item`, `add_to_cart`, `begin_checkout`, `purchase`
- **Tiny and self-contained** — ~3.7 kB gzipped, zero runtime dependencies, ESM + CJS

## Install

```sh
npm install @hitkeep/tracker
```

## Quick start

```ts
import { init, track } from '@hitkeep/tracker';

init({ host: 'https://your-hitkeep.example.com' });

track('signup_clicked', { plan: 'pro' });
```

`host` is the origin (or path prefix) your HitKeep instance serves the tracker from — the same URL the `hk.js` snippet would load from, including [custom tracking domains](https://hitkeep.com/guides/tracking/custom-tracking-domains/). Pageviews are captured automatically, including history-based SPA navigations, so no router wiring is required.

## Framework examples

### React and Next.js

```tsx
'use client';

import { useEffect } from 'react';
import { cleanup, init } from '@hitkeep/tracker';

export function Analytics() {
    useEffect(() => {
        init({ host: 'https://your-hitkeep.example.com' });
        return cleanup;
    }, []);

    return null;
}
```

### Vue and Nuxt

```ts
import { createApp } from 'vue';
import { init } from '@hitkeep/tracker';
import App from './App.vue';

init({ host: 'https://your-hitkeep.example.com' });

createApp(App).mount('#app');
```

### Angular

```ts
import { ApplicationConfig, provideAppInitializer } from '@angular/core';
import { init } from '@hitkeep/tracker';

export const appConfig: ApplicationConfig = {
    providers: [provideAppInitializer(() => init({ host: 'https://your-hitkeep.example.com' }))]
};
```

### Astro

```astro
<script>
    import { init } from '@hitkeep/tracker';

    init({ host: 'https://your-hitkeep.example.com' });
</script>
```

Full walkthroughs for each framework, including Next.js App Router and Astro view transitions, are in the [npm package guide](https://hitkeep.com/guides/tracking/npm-package/).

## API

| Export | Purpose |
| :--- | :--- |
| `init(config)` | Start the tracker and return a handle. Idempotent. |
| `track(name, properties?)` | Record a custom event. Calls before `init` are queued. |
| `trackPageview()` | Send a manual pageview. |
| `cleanup()` | Remove listeners and allow re-initialization. |
| `blockTrackingForMe()` / `enableTrackingForMe()` / `isTrackingEnabled()` | Visitor opt-out controls. |
| `trackViewItem` / `trackAddToCart` / `trackBeginCheckout` / `trackPurchase` | Typed [e-commerce events](https://hitkeep.com/guides/analytics/ecommerce/). |

### Configuration

```ts
init({
    host: 'https://your-hitkeep.example.com', // required
    autoCapturePageviews: true, // send a pageview on init
    autoTrackSpaNavigation: true, // pageviews on pushState/replaceState/popstate
    outboundLinks: true, // outbound_click events
    fileDownloads: true, // file_download events
    formSubmissions: true, // form_submit events
    declarativeEvents: false, // named clicks and submits from HTML attributes
    enablePaidSignals: false, // report the presence of a paid click signal on pageviews
    webVitals: false, // load the Web Vitals bundle from `${host}/hk-vitals.js`
    useBeacon: true, // prefer navigator.sendBeacon
    respectDoNotTrack: true, // drop tracking when DNT is enabled
    captureOnLocalhost: false, // localhost is blocked by default
    bindToWindow: true // expose window.hk.event for snippet-compatible callers
});
```

Each option maps to a `hk.js` data attribute where one exists — see the [tracker architecture guide](https://hitkeep.com/guides/tracking/tracker-architecture/) for the delivery, retry, and storage behavior shared by both.

### Named clicks and submits in HTML

Set `declarativeEvents: true` in `init()`, or add `data-enable-declarative-events="true"` to your `hk.js` script tag. Then add `data-hk-event` to a clickable element, form, or submit button. `data-hitkeep-event` is also supported.

```html
<a href="/pricing" data-hk-event="pricing_clicked">See pricing</a>
<form action="/subscribe" method="post" data-hk-event="newsletter_submitted">
    <button type="submit">Subscribe</button>
</form>
```

Use a fixed event name of 1–64 characters. Start with an ASCII letter or digit and use only ASCII letters, digits, underscores, and hyphens. The closest marker wins. On submit, the submit button's marker takes precedence over the form's marker. A marked submit button records its event when the browser dispatches the submit event, so a single click that submits a form generates one event. Marked links also support modifier and middle clicks.

When a label forwards a click to an associated control with a marker, the control handles the event.

HitKeep sends the name with the usual custom-event context and empty properties. Element text, link destinations, form fields, and other element metadata are excluded.

By default, the attributes only suppress automatic events so existing manual handlers can keep calling `track()` or `hk.event()`. When opting in, remove manual calls for interactions you want the markup to track. Named events take precedence over outbound, download, and form classification.

### Paid click signals

Set `enablePaidSignals: true` in `init()`, or add `data-enable-paid-signals="true"` to your `hk.js` script tag. Pageviews then include a boolean indicating whether the landing URL contains a nonempty `gclid` or `msclkid` parameter. Parameter names are case-sensitive. The tracker sends only the boolean and keeps it in memory across SPA pageviews, alongside existing UTM attribution.

The tracker strips query strings and fragments from referrer fields. On pages with a query string, analytics requests use keepalive fetch without an HTTP `Referer` header. Add `referrerpolicy="no-referrer"` to the snippet's script tag to apply the same protection to the browser's initial script request.

### E-commerce

```ts
import { trackPurchase } from '@hitkeep/tracker';

trackPurchase({
    transaction_id: 'tx-1042',
    value: 49.9,
    currency: 'EUR',
    items: [{ item_id: 'sku-1', item_name: 'Starter Plan', quantity: 1, price: 49.9 }]
});
```

### Visitor opt-out

This package bundles the tracker into your application, so content blockers do not filter it. Offer visitors an explicit opt-out:

```ts
import { blockTrackingForMe, enableTrackingForMe, isTrackingEnabled } from '@hitkeep/tracker';
```

The opt-out persists in `localStorage` under `hk_ignore` and is honored by both this package and the `hk.js` snippet.

## What you need to run it

The package sends data to a HitKeep instance. You can [self-host HitKeep](https://hitkeep.com/guides/installation/) as a single binary, Docker image, or Helm chart, or use [HitKeep Cloud](https://cloud.hitkeep.eu/signup?plan=free&billing=monthly&utm_source=npm&utm_medium=referral&utm_campaign=tracker_package). HitKeep is open source under the MIT license — the source for this package lives in [PascaleBeier/hitkeep](https://github.com/HitKeep/hitkeep).

## Documentation

- [NPM package guide](https://hitkeep.com/guides/tracking/npm-package/) — framework integration, full API reference, differences from the snippet
- [Tracker architecture](https://hitkeep.com/guides/tracking/tracker-architecture/) — delivery, retries, SPA handling, storage boundaries
- [Custom events](https://hitkeep.com/guides/tracking/custom-events/) — event naming and property conventions
- [Automatic events](https://hitkeep.com/guides/tracking/automatic-events/) — outbound clicks, downloads, form submits
- [E-commerce analytics](https://hitkeep.com/guides/analytics/ecommerce/) — purchase funnel tracking
- [Self-hosting HitKeep](https://hitkeep.com/guides/installation/) — binary, Docker Compose, Kubernetes

## Versioning

This package is versioned in lockstep with HitKeep itself and published with every HitKeep release. Keep the major version aligned with your HitKeep instance.

## License

MIT © Pascale Beier — see [LICENSE](https://github.com/HitKeep/hitkeep/blob/main/LICENSE).
