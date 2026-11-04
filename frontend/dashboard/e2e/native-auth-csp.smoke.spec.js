const { test, expect } = require("playwright/test");

for (const platform of ["ios", "android"]) {
    for (const decision of ["approve", "deny"]) {
        test(`native consent ${decision} follows only the registered ${platform} HTTPS callback`, async ({ page, request, baseURL }) => {
            const base = `${baseURL.replace(/\/$/, "")}/`;
            const source = await request.get(new URL("auth/native/consent?request=browser-fixture", base).toString(), { headers: { Accept: "text/html" } });
            expect(source.status()).toBe(200);
            const policy = source.headers()["content-security-policy"];
            const fixtureURL = new URL("__e2e/native-consent-csp", base).toString();
            const actionURL = new URL("api/auth/native/authorize", base).toString();
            const callback = new URL(`https://hitkeep.com/auth/native/${platform}/callback`);
            callback.searchParams.set(decision === "approve" ? "code" : "error", decision === "approve" ? "browser-fixture" : "access_denied");
            callback.searchParams.set("state", "browser-fixture");
            callback.searchParams.set("iss", "https://fixture.invalid/prefix");
            let posts = 0;
            await page.addInitScript(() => {
                window.nativeCSPViolations = [];
                document.addEventListener("securitypolicyviolation", (event) => window.nativeCSPViolations.push(event.effectiveDirective));
            });
            await page.route(fixtureURL, (route) =>
                route.fulfill({
                    contentType: "text/html",
                    headers: { "Content-Security-Policy": policy, "Referrer-Policy": "no-referrer", "Cache-Control": "no-store" },
                    body: `<form method="post" action="${actionURL}"><button name="decision" value="${decision}">${decision}</button></form>`
                })
            );
            await page.route(actionURL, async (route) => {
                expect(route.request().method()).toBe("POST");
                expect(new URLSearchParams(route.request().postData()).get("decision")).toBe(decision);
                posts++;
                await route.fulfill({ status: 303, headers: { Location: callback.toString(), "Cache-Control": "no-store" }, body: "" });
            });
            // Fulfill the exact fixed HTTPS destination without sending fixture data externally.
            await page.route(
                (url) => url.origin === callback.origin && url.pathname === callback.pathname,
                (route) => route.fulfill({ contentType: "text/html", body: "<h1>Native callback reached</h1>" })
            );
            await page.goto(fixtureURL);
            await page.getByRole("button", { name: decision, exact: true }).click();
            await expect(page).toHaveURL(callback.toString());
            await expect(page.getByRole("heading", { name: "Native callback reached" })).toBeVisible();
            expect(posts).toBe(1);
        });
    }
}
