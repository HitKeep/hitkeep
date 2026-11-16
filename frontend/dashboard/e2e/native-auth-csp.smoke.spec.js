const { test, expect } = require("playwright/test");
const { once } = require("node:events");

for (const platform of ["ios", "android"]) {
    for (const decision of ["approve", "deny"]) {
        test(`native consent ${decision} follows only the registered ${platform} HTTPS callback`, async ({ page, request, baseURL }) => {
            const base = `${baseURL.replace(/\/$/, "")}/`;
            const source = await request.get(new URL("auth/native/consent?request=browser-fixture", base).toString(), { headers: { Accept: "text/html" } });
            expect(source.status()).toBe(200);
            const policy = source.headers()["content-security-policy"];
            expect(policy).toContain("form-action 'self' https://hitkeep.com/auth/native/ios/callback https://hitkeep.com/auth/native/android/callback;");
            const fixtureURL = new URL("__e2e/native-consent-csp", base).toString();
            const actionURL = new URL("api/auth/native/authorize", base).toString();
            const callback = new URL(`https://hitkeep.com/auth/native/${platform}/callback`);
            callback.searchParams.set(decision === "approve" ? "code" : "error", decision === "approve" ? "browser-fixture" : "access_denied");
            callback.searchParams.set("state", "browser-fixture");
            callback.searchParams.set("iss", "https://fixture.invalid/prefix");
            // Playwright routes cover only the first redirect hop; Fetch pauses each hop before network access.
            await page.context().setOffline(true);
            const session = await page.context().newCDPSession(page);
            const requests = [];
            session.on("Fetch.requestPaused", ({ request }) => requests.push(`${request.method} ${request.url}`));
            await session.send("Fetch.enable", { patterns: [{ urlPattern: "*", requestStage: "Request" }] });
            try {
                const fixturePaused = once(session, "Fetch.requestPaused");
                const navigation = page.goto(fixtureURL);
                const [fixture] = await fixturePaused;
                expect(fixture.request.url).toBe(fixtureURL);
                expect(fixture.request.method).toBe("GET");
                await session.send("Fetch.fulfillRequest", {
                    requestId: fixture.requestId,
                    responseCode: 200,
                    responseHeaders: [
                        { name: "Content-Type", value: "text/html" },
                        { name: "Content-Security-Policy", value: policy },
                        { name: "Referrer-Policy", value: source.headers()["referrer-policy"] },
                        { name: "Cache-Control", value: "no-store" }
                    ],
                    body: Buffer.from(`<link rel="icon" href="data:,"><form method="post" action="${actionURL}"><button name="decision" value="${decision}">${decision}</button></form>`).toString("base64")
                });
                await navigation;
                const actionPaused = once(session, "Fetch.requestPaused");
                const click = page.getByRole("button", { name: decision, exact: true }).click();
                const [action] = await actionPaused;
                expect(action.request.url).toBe(actionURL);
                expect(action.request.method).toBe("POST");
                expect(new URLSearchParams(action.request.postData).get("decision")).toBe(decision);
                const callbackPaused = once(session, "Fetch.requestPaused");
                await session.send("Fetch.fulfillRequest", {
                    requestId: action.requestId,
                    responseCode: 303,
                    responseHeaders: [
                        { name: "Location", value: callback.toString() },
                        { name: "Cache-Control", value: "no-store" }
                    ]
                });
                const [destination] = await callbackPaused;
                expect(destination.request.url).toBe(callback.toString());
                expect(destination.request.method).toBe("GET");
                await session.send("Fetch.fulfillRequest", {
                    requestId: destination.requestId,
                    responseCode: 200,
                    responseHeaders: [{ name: "Content-Type", value: "text/html" }],
                    body: Buffer.from('<link rel="icon" href="data:,"><h1>Native callback reached</h1>').toString("base64")
                });
                await click;
                await expect(page).toHaveURL(callback.toString());
                await expect(page.getByRole("heading", { name: "Native callback reached" })).toBeVisible();
                expect(requests).toEqual([`GET ${fixtureURL}`, `POST ${actionURL}`, `GET ${callback}`]);
            } finally {
                await session.detach();
            }
        });
    }
}
