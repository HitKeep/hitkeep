const { test, expect } = require("playwright/test");
const { E2E_EMAIL, E2E_PASSWORD } = require("./support/auth");

test.describe("deployment smoke", () => {
    test("serves the dashboard shell, static assets, and public API from the configured base path", async ({ page, baseURL }) => {
        const network = collectSameOriginNetwork(page, baseURL);

        await page.goto(appUrl(baseURL, "/login"), { waitUntil: "domcontentloaded" });

        await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible();
        await expect.poll(() => page.evaluate(() => document.querySelector("base")?.getAttribute("href") || "")).toBe(publicBasePath(baseURL));

        const health = await page.request.get(appUrl(baseURL, "/healthz"));
        expect(health.ok()).toBeTruthy();

        const status = await page.request.get(appUrl(baseURL, "/api/status"));
        expect(status.ok()).toBeTruthy();
        await expectStatusPayload(status);

        await expectNoNetworkLeaks(network);
    });

    test("keeps authentication, API calls, and route refreshes inside the configured base path", async ({ page, baseURL }) => {
        const network = collectSameOriginNetwork(page, baseURL);
        const apiRequests = collectSameOriginAPIRequests(page, baseURL);

        await login(page, baseURL, "/events");

        await expectAppPath(page, baseURL, "/events");
        await expect(page.getByText("Event activity")).toBeVisible();

        await page.reload({ waitUntil: "domcontentloaded" });
        await expectAppPath(page, baseURL, "/events");
        await expect(page.getByText("Event activity")).toBeVisible();

        await page.goto(appUrl(baseURL, "/integration/api-reference"), { waitUntil: "domcontentloaded" });
        await expectAppPath(page, baseURL, "/integration/api-reference");
        await expectPrefixedAPIReferenceFrame(page, baseURL);

        const basePath = publicBasePath(baseURL);
        if (basePath !== "/") {
            expect(apiRequests.some((pathname) => pathname.startsWith(`${basePath}api/`))).toBeTruthy();
            expect(apiRequests.filter((pathname) => pathname.startsWith("/api/"))).toEqual([]);
        }

        await expectNoNetworkLeaks(network);
    });

    test("renders the reporting error icon with the packaged dashboard font", async ({ page, baseURL }) => {
        await page.route("**/api/reports", (route) => route.fulfill({ status: 503, json: {} }));
        await login(page, baseURL, "/settings/reports");

        const state = page.getByRole("region", { name: "Reports are unavailable" });
        const icon = state.locator(".page-state__icon i");
        await expect(icon).toBeVisible();
        const loaded = await icon.evaluate(async (element) => {
            const glyph = getComputedStyle(element, "::before");
            const faces = await document.fonts.load(`${glyph.fontSize} ${glyph.fontFamily}`, glyph.content);
            return faces.some((face) => face.family === "openng-icons" && face.status === "loaded");
        });
        expect(loaded).toBe(true);
        await expect(state.getByRole("button", { name: "Refresh" })).toBeVisible();
    });

    test("serves tracker assets and ingest preflight from the configured base path", async ({ request, baseURL }) => {
        const tracker = await request.get(appUrl(baseURL, "/hk.js"));
        expect(tracker.ok()).toBeTruthy();
        expect(tracker.headers()["content-type"]).toContain("javascript");

        const vitals = await request.get(appUrl(baseURL, "/hk-vitals.js"));
        expect(vitals.ok()).toBeTruthy();
        expect(vitals.headers()["content-type"]).toContain("javascript");

        const ingest = await request.fetch(appUrl(baseURL, "/ingest"), {
            method: "OPTIONS",
            headers: ingestPreflightHeaders()
        });
        expect(ingest.status()).toBe(204);
        expect(ingest.headers()["access-control-allow-origin"]).toBe("https://www.example.test");

        const webVitalsIngest = await request.fetch(appUrl(baseURL, "/ingest/web-vitals"), {
            method: "OPTIONS",
            headers: ingestPreflightHeaders()
        });
        expect(webVitalsIngest.status()).toBe(204);
        expect(webVitalsIngest.headers()["access-control-allow-origin"]).toBe("https://www.example.test");
    });
});

async function login(page, baseURL, returnPath) {
    await page.goto(appUrl(baseURL, `/login?returnUrl=${encodeURIComponent(returnPath)}`), { waitUntil: "domcontentloaded" });
    await page.locator('input[type="email"], input[name="email"]').first().fill(E2E_EMAIL);
    await page.locator('input[type="password"]').first().fill(E2E_PASSWORD);
    await page.locator('button[type="submit"]').first().click();
    await page.waitForURL((url) => !stripPublicBasePath(url.pathname, baseURL).includes("/login"), { timeout: 15_000 });
}

async function expectAppPath(page, baseURL, expectedPath) {
    await expect.poll(() => stripPublicBasePath(new URL(page.url()).pathname, baseURL)).toBe(expectedPath);
}

async function expectStatusPayload(response) {
    const payload = await response.json();
    expect(payload).toHaveProperty("needs_setup");
}

async function expectPrefixedAPIReferenceFrame(page, baseURL) {
    const frameSrc = await page.locator("iframe.api-reference-frame").first().getAttribute("src");
    expect(frameSrc).toBeTruthy();

    const url = new URL(frameSrc, new URL(baseURL).origin);
    const basePath = publicBasePath(baseURL);
    expect(url.pathname).toBe(`${basePath === "/" ? "" : basePath.replace(/\/$/, "")}/scalar/index.html` || "/scalar/index.html");
    expect(url.searchParams.get("spec")).toBe(`${basePath === "/" ? "" : basePath.replace(/\/$/, "")}/api/docs/v1/openapi.json` || "/api/docs/v1/openapi.json");
}

function appUrl(baseURL, route) {
    const base = new URL(baseURL);
    const routeURL = new URL(route, "https://hitkeep.local");
    const prefix = publicBasePath(baseURL).replace(/\/$/, "");
    base.pathname = `${prefix}${routeURL.pathname === "/" ? "" : routeURL.pathname}` || "/";
    base.search = routeURL.search;
    base.hash = routeURL.hash;
    return base.toString();
}

function publicBasePath(baseURL) {
    const pathname = new URL(baseURL).pathname;
    if (!pathname || pathname === "/") {
        return "/";
    }
    return pathname.endsWith("/") ? pathname : `${pathname}/`;
}

function stripPublicBasePath(pathname, baseURL) {
    const basePath = publicBasePath(baseURL);
    if (basePath === "/") {
        return pathname;
    }
    const prefix = basePath.replace(/\/$/, "");
    if (pathname === prefix) {
        return "/";
    }
    return pathname.startsWith(`${prefix}/`) ? pathname.slice(prefix.length) || "/" : pathname;
}

function collectSameOriginAPIRequests(page, baseURL) {
    const origin = new URL(baseURL).origin;
    const requests = [];

    page.on("request", (request) => {
        const url = new URL(request.url());
        if (url.origin === origin && url.pathname.includes("/api/")) {
            requests.push(url.pathname);
        }
    });

    return requests;
}

function collectSameOriginNetwork(page, baseURL) {
    const origin = new URL(baseURL).origin;
    const basePath = publicBasePath(baseURL);
    const failures = [];
    const escapedRequests = [];

    page.on("requestfailed", (request) => {
        const url = new URL(request.url());
        const errorText = request.failure()?.errorText || "failed";
        if (url.origin === origin && isDeploymentCriticalFailure(request) && !errorText.includes("ERR_ABORTED")) {
            failures.push(`${request.method()} ${url.pathname}: ${errorText}`);
        }
    });

    page.on("request", (request) => {
        if (basePath === "/") {
            return;
        }

        const url = new URL(request.url());
        if (url.origin !== origin) {
            return;
        }

        if (!isDeploymentCriticalResource(request)) {
            return;
        }

        if (!url.pathname.startsWith(basePath)) {
            escapedRequests.push(`${request.resourceType()} ${url.pathname}`);
        }
    });

    return { escapedRequests, failures };
}

async function expectNoNetworkLeaks(network) {
    await expect.poll(() => network.failures).toEqual([]);
    await expect.poll(() => network.escapedRequests).toEqual([]);
}

function ingestPreflightHeaders() {
    return {
        origin: "https://www.example.test",
        "access-control-request-method": "POST",
        "access-control-request-headers": "content-type"
    };
}

function isDeploymentCriticalResource(request) {
    return ["script", "stylesheet", "fetch", "xhr", "image"].includes(request.resourceType());
}

function isDeploymentCriticalFailure(request) {
    return ["script", "stylesheet", "fetch", "xhr"].includes(request.resourceType());
}
