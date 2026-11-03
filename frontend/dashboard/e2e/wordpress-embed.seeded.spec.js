const { test, expect } = require("playwright/test");
const { login } = require("./support/auth");

for (const width of [1440, 390]) {
    test(`WordPress embed uses native charts and read-only credentials at ${width}px`, async ({ page }, testInfo) => {
        await page.setViewportSize({ width, height: 1000 });
        await login(page);
        const bootstrap = await (await page.request.get("/api/user/bootstrap")).json();
        const site = bootstrap.sites.find((site) => site.domain === "acme-analytics.io");
        expect(site).toBeTruthy();
        const shell = await (await page.request.get("/login")).text();
        const origin = new URL(page.url()).origin;
        // Only the broker/signature are fixtures: the iframe runs the real dashboard and chart components.
        const launch = `fixture.${Buffer.from(JSON.stringify({ site_id: site.id, parent_origin: origin, exp: Math.floor(Date.now() / 1000) + 300 })).toString("base64url")}.fixture`;
        const frameURL = `/embed/${site.id}/dashboard?launch=${launch}`;
        const reads = [];
        await page.route("**/embed/**", (route) => route.fulfill({ contentType: "text/html", body: shell }));
        await page.route("**/api/**", async (route) => {
            const request = route.request();
            const headers = await request.allHeaders();
            expect(request.method()).toBe("GET");
            expect(headers.cookie).toBeUndefined();
            expect(headers.authorization).toBe("Bearer hk_embed_fixture");
            expect(headers["x-hitkeep-embed-parent"]).toBe(origin);
            const path = new URL(request.url()).pathname;
            reads.push(path);
            if (path === `/api/embed/sites/${site.id}/site`) {
                await route.fulfill({ json: { ...site, role: "viewer" } });
                return;
            }
            expect(path.startsWith(`/api/sites/${site.id}/`)).toBeTruthy();
            // The seeded account supplies aggregate fixture data; Go tests prove actual bearer enforcement.
            const response = await page.request.get(request.url());
            expect(response.ok()).toBeTruthy();
            await route.fulfill({ response });
        });
        await page.route("**/wordpress-embed-fixture", (route) =>
            route.fulfill({
                contentType: "text/html",
                body: `<script>addEventListener('message', event => {
                const frame = document.querySelector('iframe');
                if (event.origin !== location.origin || event.source !== frame.contentWindow || event.data.type !== 'hitkeep:embed:ready') return;
                frame.contentWindow.postMessage({type:'hitkeep:embed:token',site_id:${JSON.stringify(site.id)},nonce:event.data.nonce,token:'hk_embed_fixture',expires_at:new Date(Date.now()+300000).toISOString()}, location.origin);
            });</script><iframe title="WordPress dashboard" style="width:100%;height:950px;border:0" src="${frameURL}"></iframe>`
            })
        );
        await page.goto("/wordpress-embed-fixture");
        const frame = page.frameLocator("iframe");
        await expect(frame.getByText("Read-only analytics", { exact: true })).toBeVisible();
        await expect(frame.locator("app-traffic-chart canvas").first()).toBeVisible();
        await expect(frame.locator("app-layout-sidebar")).toHaveCount(0);
        await expect(frame.locator("app-site-selector")).toHaveCount(0);
        const range = frame.getByRole("button", { name: "7d", exact: true }).first();
        await range.click();
        await expect(range).toHaveAttribute("aria-pressed", "true");
        await frame.getByRole("link", { name: "Ecommerce", exact: true }).click();
        await expect(frame.locator("app-ecommerce")).toBeVisible();
        await expect(frame.getByRole("button", { name: /export/i })).toHaveCount(0);
        expect(reads.some((path) => path.endsWith("/stats"))).toBeTruthy();
        expect(reads.some((path) => path.endsWith("/ecommerce"))).toBeTruthy();
        await page.screenshot({ path: testInfo.outputPath(`wordpress-embed-${width}.png`), fullPage: true });
    });
}
