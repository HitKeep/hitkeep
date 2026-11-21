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
- **Self-contained** — zero runtime dependencies, ESM + CJS

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
    visibleTime: false, // measure time the page is visible
    useBeacon: true, // prefer navigator.sendBeacon
    respectDoNotTrack: true, // drop tracking when DNT is enabled
    captureOnLocalhost: false, // localhost is blocked by default
    bindToWindow: true // expose window.hk.event for snippet-compatible callers
});
```

Each option maps to a `hk.js` data attribute where one exists — see the [tracker architecture guide](https://hitkeep.com/guides/tracking/tracker-architecture/) for the delivery, retry, and storage behavior shared by both.

### Visible time

Set `visibleTime: true` in `init()`, or add `data-enable-visible-time="true"` to your `hk.js` script tag. HitKeep measures how long each tracked pageview is visible, up to 24 hours. Timing pauses when the page is hidden, and a back/forward cache restore continues the original pageview.

Each pageview can send up to two checkpoints when the page is hidden or cached, plus a final sample when navigation ends the view. Each sample contains the pageview ID and cumulative milliseconds. Unchanged values are skipped, and SDK `cleanup()` discards any pending measurement. An interrupted exit or failed request can leave a pageview with a partial measurement or no measurement.

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

### Track page-not-found views

Use the fixed event name `page_not_found` to count views of your 404 page. Each example uses your existing tracker installation and sends one named event with empty properties per eligible 404 view, in addition to normal pageview tracking.

Choose one example for each view. In the site's **Events** report, select `page_not_found` to see its occurrence count. A server can return HTTP 404 while a client router can show its missing route after loading an HTTP 200 app shell.

Use these examples only where the current and previous tracked pathnames are safe to collect. Both examples skip the event when the document's incoming referrer includes anything beyond the origin.

#### Server-rendered 404 page

In the template your server returns with HTTP status 404, register the listener before your existing snippet. The window's `load` event waits for the `async` tracker script. Keep the URL and any `data-*` options from your existing snippet, and add the listener only to the 404 template. The `referrerpolicy` attribute keeps the current URL out of the request for `hk.js`.

```html
<script>
    window.addEventListener('load', () => {
        const referrer = document.referrer;
        if (!referrer || referrer === `${new URL(referrer).origin}/`) {
            window.hk?.event?.('page_not_found');
        }
    }, { once: true });
</script>
<script async src="https://your-hitkeep.example.com/hk.js" referrerpolicy="no-referrer"></script>
```

#### Vue Router SPA

Give the catch-all route a name and register the hook once in your browser entry point, before `app.use(router)` starts the initial navigation. Vue Router reports duplicate and aborted navigations through `failure`. A query or fragment change on the same missing pathname keeps the current 404 view. A different missing pathname or a later return to a missing pathname counts as a new view.

```ts
import { createApp } from 'vue';
import { createRouter, createWebHistory } from 'vue-router';
import { init, track } from '@hitkeep/tracker';
import App from './App.vue';
import Home from './Home.vue';
import NotFound from './NotFound.vue';

const router = createRouter({
    history: createWebHistory(),
    routes: [
        { path: '/', component: Home },
        { path: '/:pathMatch(.*)*', name: 'not-found', component: NotFound }
    ]
});

init({ host: 'https://your-hitkeep.example.com' });

const referrer = document.referrer;
const hasPublicReferrer = !referrer || referrer === `${new URL(referrer).origin}/`;
router.afterEach((to, from, failure) => {
    if (failure || to.name !== 'not-found' || !hasPublicReferrer) return;
    if (from.name === to.name && from.path === to.path) return;

    track('page_not_found');
});

createApp(App).use(router).mount('#app');
```

#### Keep private URLs out of tracking

The example checks the incoming `document.referrer` before calling the tracker. The tracker sends the current pathname as `path` and the previous SPA URL or incoming referrer as `r`, with query strings and fragments removed. The `p` object for custom properties is empty. Normal pageviews can still send referrer context and recognized campaign parameters when the example skips the event.

Set `Referrer-Policy: no-referrer` on pages that link from sensitive URLs, including same-origin pages, or use `rel="noreferrer"` on those links. Set the header on the tracked response as well to keep the current URL out of the analytics request's HTTP `Referer` header. A policy on the 404 response alone leaves its incoming `document.referrer` intact.

Where a 404 pathname or incoming referrer can contain private data, serve that response without the tracker. For an SPA, keep tracking in a public shell and send visitors to a separate, untracked document for private routes, since automatic pageviews also read the URL.

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
