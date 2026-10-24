#!/usr/bin/env node
/**
 * Curated HitKeep docs and README screenshot set.
 *
 * Run through `./hk screenshot docs`, which starts from a ready, seeded
 * development session and passes the dashboard URL, the seeded credentials,
 * and a run artifact directory. `--sync` then copies the images into the
 * README assets and the adjacent private docs repository.
 *
 * Environment (set by hk):
 *   HITKEEP_URL        Dashboard base URL
 *   HITKEEP_EMAIL      Seeded account email
 *   HITKEEP_PASSWORD   Seeded account password
 *   OUTPUT_DIR         Output directory (required)
 *   SCALE              Device pixel ratio (default: 2)
 *   SCREENSHOT_TARGET  Optional subset, e.g. "ask-ai"
 *
 * For quick visual QA of arbitrary routes, use `./hk screenshot /route`.
 */

import { mkdirSync, readFileSync, readdirSync, writeFileSync } from "fs";
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { join, resolve } from "path";

import { chromium } from "playwright";

const BASE_URL = (process.env.HITKEEP_URL ?? "http://localhost:8080").replace(/\/$/, "");
const EMAIL = process.env.HITKEEP_EMAIL;
const PASSWORD = process.env.HITKEEP_PASSWORD;
const SCALE = parseFloat(process.env.SCALE ?? "2");
const DESKTOP_VIEWPORT = { width: 1440, height: 1024 };
const MOBILE_VIEWPORT = { width: 390, height: 844 };
// The shared metric card grid is three columns from 1280px up
// (frontend/dashboard/src/app/features/analytics/components/metric-card-group.css),
// so two columns keep the AI Agents grid's four groups a gap-free 2x2.
const AI_CORRELATION_VIEWPORT = { width: 1180, height: 1024 };
const SCREENSHOT_TARGET = (process.env.SCREENSHOT_TARGET ?? "").trim().toLowerCase();
const OUTPUT_DIR = process.env.OUTPUT_DIR ? resolve(process.env.OUTPUT_DIR) : "";
const SCRIPT_SHA256 = createHash("sha256")
    .update(readFileSync(new URL(import.meta.url)))
    .digest("hex");

if (!EMAIL || !PASSWORD || !OUTPUT_DIR) {
    console.error("\n  Error: HITKEEP_EMAIL, HITKEEP_PASSWORD, and OUTPUT_DIR are required; run ./hk screenshot docs.\n");
    process.exit(1);
}

const CHART_SETTLE = 2500;
const TABLE_SETTLE = 1000;
const FORM_SETTLE = 600;
const DEMO_ASK_AI_STATUS = {
    enabled: true,
    available: true,
    status: "available",
    provider: "openai-compatible",
    model: "gpt-4.1-mini",
    budget_exhausted: false
};
const DEMO_SITE_DOMAIN = process.env.HITKEEP_SCREENSHOT_SITE ?? "acme-analytics.io";
// Full runs must replace the entire owned set; omissions must never sync stale assets.
const EXPECTED_SCREENSHOTS =
    `admin-sites-list admin-system-activation admin-system-audit admin-system-operations admin-system-status admin-team-api-clients admin-team-audit admin-team-custom-domains admin-team-members admin-team-overview admin-team-settings analytics-ai-agents-traffic analytics-ai-chatbots analytics-ai-visibility-correlation analytics-ai-visibility analytics-annotations-dialog analytics-annotations analytics-ecommerce analytics-events-audience analytics-events analytics-funnels analytics-goals analytics-opportunities analytics-search-console-mobile analytics-search-console analytics-utm analytics-web-vitals-rating-pages analytics-web-vitals dashboard-comparison dashboard-overview dashboard-sites-overview feature-ask-ai-answer feature-ask-ai-budget-selfhost-mobile feature-ask-ai-budget-selfhost feature-ask-ai-empty feature-ask-ai-mobile feature-create-team feature-custom-domain-setup feature-email-reports-mobile feature-email-reports feature-onboarding-checklist feature-report-email-preview feature-share-dashboard feature-shared-table feature-site-tracking feature-site-transfer feature-team-switcher feature-theme-dialog feature-tracking-verifier global_filters imports-plausible-events-report imports-plausible-provider imports-plausible-zip-review imports-simple-analytics-upload integration-api-reference integration-google-search-console-mobile integration-google-search-console integration-webhook-deliveries integration-webhook-form integration-webhooks-events integration-webhooks mcp oauth-client-registration oauth-clients oauth-client-details oauth-connected-applications oauth-application-details page-login qr-campaigns-analytics qr-campaigns-editor qr-campaigns-list qr-campaigns-share-links release-2-5-1-api-client-grants-dialog release-2-5-1-api-clients-card release-2-5-1-api-clients-mobile release-2-5-1-opportunities-dark release-2-5-1-team-api-clients security-2fa-setup security-api-clients security-sso-login security-sso-team-configuration session-expiry-warning settings-profile social-sign-in-providers team-traffic-exclusions-inherited tools-utm-builder wordpress-plugin-oauth-consent`.split(
        /\s+/
    );

function escapeRegExp(value) {
    return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

async function login(page) {
    await page.goto(`${BASE_URL}/login`, { waitUntil: "domcontentloaded", timeout: 20_000 });
    await page.waitForSelector('input[type="password"]', { state: "visible", timeout: 10_000 });

    await page.locator('input[type="email"], input[name="email"]').first().fill(EMAIL);
    await page.locator('input[type="password"]').fill(PASSWORD);

    await page.locator('button[type="submit"]').click();
    await page.waitForURL((url) => !url.pathname.includes("/login"), { timeout: 12_000 });
}

async function nav(page, path, settle = TABLE_SETTLE) {
    await page.goto(`${BASE_URL}${path}`, { waitUntil: "domcontentloaded", timeout: 20_000 });
    await page.evaluate(() => document.querySelector("main > div")?.scrollTo(0, 0));
    await page.waitForFunction(() => !document.fonts || document.fonts.status === "loaded", null, { timeout: 3_000 }).catch(() => {});
    await page.evaluate(() => new Promise((resolveFrame) => requestAnimationFrame(() => requestAnimationFrame(resolveFrame))));
    await page.waitForTimeout(settle);
}

async function clickTab(page, label, settle = TABLE_SETTLE) {
    const tab = page.getByRole("tab", { name: new RegExp(label, "i") }).first();
    const teamSections = { members: "members", "api clients": "api-clients", "custom domains": "custom-domains", branding: "branding", activity: "activity" };
    if (!(await tab.count()) && teamSections[label.toLowerCase()]) {
        await nav(page, `/admin/team/${teamSections[label.toLowerCase()]}`, settle);
        return true;
    }
    if (!(await tab.count())) {
        console.warn(`    ! Tab not found: ${label}`);
        return false;
    }
    await tab.click();
    await page.waitForFunction((tabLabel) => {
        const tabs = Array.from(document.querySelectorAll('[role="tab"]'));
        const matched = tabs.find((el) => new RegExp(tabLabel, "i").test(el.textContent || ""));
        return matched?.getAttribute("aria-selected") === "true";
    }, label);
    await page.waitForTimeout(settle);
    return true;
}

async function selectRangePreset(page, label, settle = CHART_SETTLE) {
    const toolbar = page.locator("app-range-toolbar").first();
    const option = toolbar.getByRole("button", { name: new RegExp(`^${escapeRegExp(label)}$`) }).first();
    if (!(await option.count())) {
        console.warn(`    ! Range preset not found: ${label}`);
        return false;
    }
    await option.click();
    await page.waitForTimeout(settle);
    return true;
}

async function shoot(page, slug, { fullPage = false, clip } = {}) {
    const file = join(OUTPUT_DIR, `${slug}.png`);
    try {
        await page.waitForFunction(() => !Array.from(document.querySelectorAll("p-skeleton, [role='progressbar'], .pi-spin, [aria-busy='true']")).some((el) => el.getClientRects().length), null, { timeout: 20_000 });
        await page.screenshot({ path: file, fullPage, clip, animations: "disabled" });
        return {
            ok: true,
            file,
            route: new URL(page.url()).pathname,
            viewport: page.viewportSize(),
            sha256: createHash("sha256").update(readFileSync(file)).digest("hex"),
            theme: await page.locator("html").evaluate((el) => (el.classList.contains("p-dark") ? "dark" : "light"))
        };
    } catch (err) {
        return { ok: false, file, error: err.message };
    }
}

async function installAskAIDemoRoutes(page) {
    await page.route("**/api/user/bootstrap", async (route) => {
        const response = await route.fetch();
        const body = await response.json();
        body.status = body.status || {};
        body.status.ai = { ...(body.status.ai || {}), enabled: true, configured: true, ask_ai_enabled: true, ask_ai_available: true };
        body.status.ask_ai = DEMO_ASK_AI_STATUS;
        body.assistants = { enabled: true, endpoint: "https://analytics.example.com/mcp" };
        await route.fulfill({ response, json: body });
    });
    // The drawer prefers the per-site status over bootstrap; demo it as available.
    await page.route("**/api/sites/*/ask-ai/status", (route) => route.fulfill({ json: DEMO_ASK_AI_STATUS }));

    await page.route("**/api/sites/*/ask-ai/events", async (route) => {
        // Mirrors the live flow: the server reads the snapshot (overview and
        // notes) before the model answers, the model calls the breakdown for the
        // change drivers, the answer streams, and the server cites the tools that
        // ran by name with English titles the drawer translates.
        const tool = (status, name, messageKey) => ({ type: "progress", status, message_key: messageKey, tool_call_id: `demo-${name}`, tool_name: name });
        const read = (name) => [tool("tool_call_start", name, "askAi.progress.readingAnalytics"), tool("tool_call_finish", name, "askAi.progress.composing")];
        const answer = [
            "In this demo site, pageviews rose **21%** compared with last month, from 4,120 to 4,985.\n\n",
            "- **news.ycombinator.com** drove most of it: +612 pageviews, 71% of the change.\n",
            "- The team noted the **pricing page redesign** on September 12, which lines up with the rise."
        ];
        const events = [
            { type: "progress", status: "accepted", message_key: "askAi.progress.accepted" },
            ...read("hitkeep_get_annotations"),
            ...read("hitkeep_get_site_overview"),
            ...read("hitkeep_get_breakdown"),
            ...answer.map((chunk) => ({ type: "delta", status: "streaming", delta_markdown: chunk })),
            {
                type: "final",
                status: "success",
                message_key: "askAi.progress.complete",
                response: {
                    run_id: "ask-ai-screenshot-demo",
                    answer_markdown: answer.join(""),
                    citations: [
                        { label: "Get HitKeep Annotations", tool_call_id: "hitkeep_get_annotations" },
                        { label: "Get HitKeep Breakdown", tool_call_id: "hitkeep_get_breakdown" },
                        { label: "Get HitKeep Site Overview", tool_call_id: "hitkeep_get_site_overview" }
                    ],
                    charts: [
                        {
                            type: "table",
                            title: "Biggest movers by referrer",
                            rows: [
                                { referrer: "news.ycombinator.com", last_month: 88, this_month: 700, change: 612 },
                                { referrer: "(Direct)", last_month: 2300, this_month: 2443, change: 143 },
                                { referrer: "google.com", last_month: 1732, this_month: 1842, change: 110 }
                            ]
                        }
                    ],
                    actions: [{ type: "navigate", label: "Open the dashboard", target: "/dashboard" }]
                }
            }
        ];
        const body = events.map((event) => `event: ${event.type}\ndata: ${JSON.stringify(event)}\n`).join("\n");
        await route.fulfill({
            status: 200,
            headers: {
                "Cache-Control": "no-cache",
                "Content-Type": "text/event-stream; charset=utf-8"
            },
            body: `${body}\n`
        });
    });
}

async function maybeSelectFirstOption(page, comboLabel) {
    const combo = page.getByRole("combobox", { name: new RegExp(comboLabel, "i") }).first();
    if (!(await combo.count())) return false;
    if (!(await combo.isEnabled())) return false;
    if ((await combo.getAttribute("aria-disabled")) === "true") return false;

    await combo.click({ timeout: 1500 });
    const option = page.getByRole("option").first();
    if (!(await option.count())) return false;

    await option.click();
    await page.waitForTimeout(TABLE_SETTLE);
    return true;
}

async function waitForSelectEnabled(page, selector, timeout = 8_000) {
    await page.waitForFunction(
        (cssSelector) => {
            const el = document.querySelector(cssSelector);
            if (!el) return false;
            const ariaDisabled = el.getAttribute("aria-disabled");
            return ariaDisabled !== "true" && !el.classList.contains("p-disabled");
        },
        selector,
        { timeout }
    );
}

async function selectFirstVisibleOption(page, selector) {
    const select = page.locator(`${selector}:visible`).first();
    if (!(await select.count())) return false;
    if (!(await select.isEnabled())) return false;
    if ((await select.getAttribute("aria-disabled")) === "true") return false;

    await select.click({ timeout: 2_000 });

    await page.waitForSelector('[role="option"]:visible', { timeout: 5_000 });
    const option = page.locator('[role="option"]:visible').first();
    if (!(await option.count())) return false;

    const optionText = ((await option.textContent()) ?? "").trim();
    await option.click();
    await page.waitForTimeout(TABLE_SETTLE);

    if (optionText) {
        try {
            await page.waitForFunction(
                ({ cssSelector, expectedText }) => {
                    const el = document.querySelector(cssSelector);
                    return !!el && (el.textContent || "").includes(expectedText);
                },
                { cssSelector: selector, expectedText: optionText },
                { timeout: 5_000 }
            );
        } catch {
            // OptimusUI may render the selected label outside the host element.
        }
    }

    return true;
}

async function selectVisibleOptionMatching(page, selector, pattern) {
    const select = page.locator(`${selector}:visible`).first();
    if (!(await select.count())) return false;
    if (!(await select.isEnabled())) return false;
    if ((await select.getAttribute("aria-disabled")) === "true") return false;

    await select.click({ timeout: 2_000 });
    await page.waitForSelector('[role="option"]:visible', { timeout: 5_000 });

    const options = page.locator('[role="option"]:visible');
    const count = await options.count();
    for (let i = 0; i < count; i += 1) {
        const option = options.nth(i);
        const optionText = ((await option.textContent()) ?? "").trim();
        if (!pattern.test(optionText)) continue;

        await option.click();
        await page.waitForTimeout(TABLE_SETTLE);
        try {
            await page.waitForFunction(
                ({ cssSelector, expectedText }) => {
                    const el = document.querySelector(cssSelector);
                    return !!el && (el.textContent || "").includes(expectedText);
                },
                { cssSelector: selector, expectedText: optionText.replace(/\s*Auto\s*$/i, "") },
                { timeout: 5_000 }
            );
        } catch {
            // OptimusUI may render the selected label outside the host element.
        }
        return true;
    }

    await page.keyboard.press("Escape");
    return false;
}

async function prepareEventBreakdown(page) {
    const eventSelector = "#event-name-select";
    const propertySelector = "#property-key-select";

    if (!(await selectVisibleOptionMatching(page, eventSelector, /outbound_click/i)) && !(await selectFirstVisibleOption(page, eventSelector))) {
        console.warn("    ! Event selector could not pick a value");
        return false;
    }

    await waitForSelectEnabled(page, propertySelector);

    if (!(await selectFirstVisibleOption(page, propertySelector))) {
        console.warn("    ! Property selector could not pick a value");
        return false;
    }

    await page.waitForTimeout(CHART_SETTLE);
    return true;
}

async function prepareAIVisibilityShot(page) {
    const tables = page.locator('[data-testid="metric-card-group-correlation"]').first();
    if (await tables.count()) {
        // The caller restores the viewport: this clip is measured at this width, so
        // the shot has to be taken before anything reflows it.
        await page.setViewportSize(AI_CORRELATION_VIEWPORT);
        await tables.scrollIntoViewIfNeeded();
        await page.waitForTimeout(TABLE_SETTLE);

        const clip = await page.evaluate(() => {
            const tables = document.querySelector('[data-testid="metric-card-group-correlation"]');
            const card = document.querySelector('[data-testid="ai-visibility-fetch-depth"]');
            const grid = document.querySelector('[data-testid="metric-card-group"]');

            if (!tables || !card || !grid) {
                return null;
            }

            const gridRect = grid.getBoundingClientRect();
            const tablesRect = tables.getBoundingClientRect();
            const cardRect = card.getBoundingClientRect();

            const horizontalPadding = 18;
            const verticalPadding = 18;
            const scrollX = window.scrollX;
            const scrollY = window.scrollY;
            const absLeft = gridRect.left + scrollX;
            const absRight = gridRect.right + scrollX;
            const absTop = tablesRect.top + scrollY;
            const absBottom = cardRect.bottom + scrollY;
            const documentWidth = Math.max(document.documentElement.scrollWidth, document.body.scrollWidth);
            const documentHeight = Math.max(document.documentElement.scrollHeight, document.body.scrollHeight);

            const x = Math.max(absLeft - horizontalPadding, 0);
            const y = Math.max(absTop - verticalPadding, 0);
            const width = Math.max(Math.min(absRight + horizontalPadding, documentWidth) - x, 1);

            // Keep this asset landscape-oriented so it reads like a marketing hero instead of a document scan.
            const preferredHeight = Math.min(Math.floor(window.innerHeight * 0.76), 760);
            const maxBottom = Math.min(absBottom + verticalPadding, documentHeight);
            const height = Math.max(Math.min(maxBottom - y, preferredHeight), 1);

            return {
                x,
                y,
                width,
                height
            };
        });

        if (clip) {
            return clip;
        }
    }

    await page.evaluate(() => window.scrollTo({ top: Math.floor(window.innerHeight * 1.2), behavior: "instant" }));
    await page.waitForTimeout(TABLE_SETTLE);
    return null;
}

async function captureRoute(page, record, slug, path, settle = TABLE_SETTLE) {
    await nav(page, path, settle);
    if (path === "/dashboard") await selectRangePreset(page, "30d");
    if (path === "/settings/reports") await page.getByTestId("report-table").waitFor({ state: "visible" });
    if (path === "/import-export/import") await page.getByRole("button", { name: /select plausible importer/i }).waitFor({ state: "visible" });
    record(slug, await shoot(page, slug));
}

async function selectSiteByDomain(page, domain = DEMO_SITE_DOMAIN) {
    const sites = await page.evaluate(async () => (await fetch("/api/sites")).json());
    const site = sites.find((site) => site.domain === domain);
    if (!site) throw new Error(`Seeded screenshot site not found: ${domain}`);
    // Reuse the app's persisted selection, as the seeded QR tests do.
    const selected = await page.evaluate(() => localStorage.getItem("hk_last_site_id"));
    if (selected !== site.id) {
        await page.evaluate((id) => localStorage.setItem("hk_last_site_id", id), site.id);
        await page.reload({ waitUntil: "domcontentloaded" });
        await page.waitForTimeout(CHART_SETTLE);
    }
    await page
        .locator("main > div")
        .first()
        .evaluate((el) => el.scrollTo(0, 0));
    return true;
}

async function captureSearchConsoleDrilldown(page, record, slug) {
    await nav(page, "/dashboard", CHART_SETTLE);
    await selectSiteByDomain(page);
    await selectRangePreset(page, "7d", CHART_SETTLE);
    const drilldown = page.locator('[data-testid="search-console-drilldown"]').first();
    if (!(await drilldown.count())) {
        console.warn("    ! Search Console drilldown not found, skipping screenshot");
        return;
    }
    await drilldown.evaluate((el) => el.scrollIntoView({ behavior: "instant", block: "start" }));
    await page.waitForSelector('[data-testid="search-console-setup-prompt"]', { state: "detached", timeout: 8_000 }).catch(() => {});
    await page
        .getByText(/privacy friendly analytics|self hosted web analytics|cookie free analytics/i)
        .first()
        .waitFor({ state: "visible", timeout: 8_000 })
        .catch(() => {
            console.warn("    ! Search Console data rows were not visible before capture");
        });
    await page.waitForTimeout(TABLE_SETTLE);
    record(slug, await shoot(page, slug));
}

async function captureOpportunities(page, record, slug) {
    await nav(page, "/opportunities", TABLE_SETTLE);
    await selectSiteByDomain(page);
    await page
        .locator(".opportunities-inbox")
        .first()
        .waitFor({ state: "visible", timeout: 15_000 })
        .catch(() => {
            console.warn("    ! Opportunity inbox was not visible before capture");
        });
    const firstCard = page.locator("app-opportunity-card").first();
    await firstCard.waitFor({ state: "visible", timeout: 15_000 }).catch(() => {
        console.warn("    ! Opportunity cards were not visible before capture");
    });
    record(slug, await shoot(page, slug));
}

async function captureWebVitals(page, record) {
    await nav(page, "/web-vitals", CHART_SETTLE);
    record("analytics-web-vitals", await shoot(page, "analytics-web-vitals"));

    const poorButton = page.getByRole("button", { name: /poor/i }).first();
    if (!(await poorButton.count())) {
        console.warn("    ! Web Vitals poor rating control not found, skipping drilldown screenshot");
        return;
    }

    await poorButton.click();
    await page.getByRole("heading", { name: /poor .* pages/i }).waitFor({ state: "visible", timeout: 8_000 });
    await page.evaluate(() => {
        const heading = Array.from(document.querySelectorAll("h3")).find((el) => /poor .* pages/i.test(el.textContent || ""));
        heading?.scrollIntoView({ behavior: "instant", block: "start" });
    });
    await page.waitForTimeout(TABLE_SETTLE);
    record("analytics-web-vitals-rating-pages", await shoot(page, "analytics-web-vitals-rating-pages"));
}

async function captureGoogleSearchConsoleIntegration(page, record, slug) {
    await nav(page, "/integration/google-search-console", FORM_SETTLE);
    await selectSiteByDomain(page);
    await page
        .getByText(/sc-domain:acme-analytics\.io/i)
        .first()
        .waitFor({ state: "visible", timeout: 8_000 })
        .catch(() => {
            console.warn("    ! Mapped Search Console property was not visible before capture");
        });
    record(slug, await shoot(page, slug));
}

async function openAskAIDrawer(page) {
    await nav(page, "/dashboard", CHART_SETTLE);
    await selectSiteByDomain(page);
    if (!(await page.locator(".ask-ai-trigger:visible").first().count())) {
        const mobileMenu = page.getByRole("button", { name: /^open navigation$/i }).first();
        if (await mobileMenu.count()) {
            await mobileMenu.click();
            await page.waitForTimeout(FORM_SETTLE);
        }
    }
    const trigger = page.locator(".ask-ai-trigger:visible").first();
    await trigger.waitFor({ state: "visible", timeout: 10_000 });
    await trigger.click();
    await page.locator(".ask-ai-drawer").waitFor({ state: "visible", timeout: 8_000 });
    await page.waitForTimeout(FORM_SETTLE);
}

async function captureAskAI(page, record) {
    await openAskAIDrawer(page);
    await page.locator(".ai-suggestions").waitFor({ state: "visible", timeout: 8_000 });
    record("feature-ask-ai-empty", await shoot(page, "feature-ask-ai-empty"));

    const prompt = "Why did pageviews rise compared with last month?";
    await page.locator('[name="ask-ai-panel-query"]').fill(prompt);
    await page.getByRole("button", { name: /send ask ai question/i }).click();
    await page.getByText(/In this demo site, pageviews rose/i).waitFor({ state: "visible", timeout: 8_000 });
    await page.getByText(/Biggest movers by referrer/i).waitFor({ state: "visible", timeout: 8_000 });
    await page.waitForTimeout(FORM_SETTLE);
    record("feature-ask-ai-answer", await shoot(page, "feature-ask-ai-answer"));

    await page.setViewportSize(MOBILE_VIEWPORT);
    await openAskAIDrawer(page);
    await page.locator(".ai-suggestions").waitFor({ state: "visible", timeout: 8_000 });
    await page.waitForTimeout(FORM_SETTLE);
    record("feature-ask-ai-mobile", await shoot(page, "feature-ask-ai-mobile"));
    await page.setViewportSize(DESKTOP_VIEWPORT);
}

/** Demo notes relative to today, so the chart always shows the same story on a 30-day range. */
function demoAnnotations() {
    const day = (daysAgo) => {
        const date = new Date();
        date.setUTCHours(0, 0, 0, 0);
        date.setUTCDate(date.getUTCDate() - daysAgo);
        return date.toISOString();
    };
    const note = (id, startsAgo, body, endsAgo) => ({ id, site_id: "demo", starts_at: day(startsAgo), ...(endsAgo === undefined ? {} : { ends_at: day(endsAgo) }), body, created_at: day(startsAgo) });
    return [note("demo-campaign", 22, "Autumn newsletter campaign", 17), note("demo-pricing", 13, "Launched the new pricing page"), note("demo-outage", 6, "CDN outage (40 min)"), note("demo-launch", 2, "Product Hunt launch")];
}

async function captureAnnotations(page, record) {
    const pattern = "**/api/sites/*/annotations*";
    const handler = (route) => (route.request().method() === "GET" ? route.fulfill({ json: demoAnnotations() }) : route.continue());
    await page.route(pattern, handler);
    try {
        await nav(page, "/dashboard", CHART_SETTLE);
        await selectSiteByDomain(page);
        await selectRangePreset(page, "30d");
        const chart = page.locator("app-traffic-chart").first();
        await chart.locator("app-annotation-strip li").first().waitFor({ state: "visible", timeout: 8_000 });

        // Clip to the traffic card: the nearest bordered, rounded ancestor of the chart.
        const clip = await chart.evaluate((element) => {
            let card = element;
            while (card.parentElement) {
                const style = getComputedStyle(card);
                if (parseFloat(style.borderTopWidth) > 0 && parseFloat(style.borderTopLeftRadius) > 0) break;
                card = card.parentElement;
            }
            card.scrollIntoView({ block: "center", behavior: "instant" });
            const rect = card.getBoundingClientRect();
            const pad = 16;
            return { x: Math.max(0, rect.x - pad), y: Math.max(0, rect.y - pad), width: rect.width + pad * 2, height: rect.height + pad * 2 };
        });
        await page.waitForTimeout(FORM_SETTLE);
        record("analytics-annotations", await shoot(page, "analytics-annotations", { clip }));

        // Drag across a few days to show the range gesture ending in the note dialog.
        const canvas = chart.locator("canvas").first();
        const box = await canvas.boundingBox();
        if (box) {
            const y = box.y + box.height * 0.55;
            await page.mouse.move(box.x + box.width * 0.62, y);
            await page.mouse.down();
            await page.mouse.move(box.x + box.width * 0.7, y, { steps: 6 });
            await page.mouse.move(box.x + box.width * 0.76, y, { steps: 6 });
            await page.mouse.up();
            const dialog = page.getByRole("dialog");
            await dialog.waitFor({ state: "visible", timeout: 8_000 });
            await page.locator("#annotation-body").fill("Spring sale landing page");
            await page.waitForTimeout(FORM_SETTLE);
            record("analytics-annotations-dialog", await shoot(page, "analytics-annotations-dialog"));
            await page.keyboard.press("Escape");
            await dialog.waitFor({ state: "hidden", timeout: 8_000 }).catch(() => {});
        }
    } finally {
        await page.unroute(pattern, handler);
    }
}

async function captureReportEmailPreview(page, record) {
    await nav(page, "/settings/reports", FORM_SETTLE);
    await page.getByRole("button", { name: "New report" }).first().click();
    const firstStep = page.locator(".editor-step").first();
    await firstStep.waitFor({ state: "visible", timeout: 8_000 });
    await firstStep.locator("input").first().fill("Weekly site summary");
    const preview = page.locator('[data-testid="report-email-preview"]');
    await preview.waitFor({ state: "visible", timeout: 10_000 });
    await page.waitForTimeout(FORM_SETTLE);
    // Frame the preview step: its heading at the top, the envelope and email header below.
    await page
        .locator(".editor-step")
        .last()
        .evaluate((step) => step.scrollIntoView({ block: "start", behavior: "instant" }));
    await page.waitForTimeout(400);
    record("feature-report-email-preview", await shoot(page, "feature-report-email-preview"));
    await page.keyboard.press("Escape");
    await page.waitForTimeout(400);
}

async function captureThemeDialog(page, record) {
    await nav(page, "/dashboard", CHART_SETTLE);
    await selectSiteByDomain(page);
    await page.getByRole("button", { name: "Open user menu", exact: true }).click();
    await page.getByRole("menuitem", { name: "Customize theme", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await dialog.waitFor({ state: "visible", timeout: 8_000 });
    // Theme changes preview live; pick a preset so the dashboard behind shows it, then discard.
    const teal = dialog.getByRole("button", { name: /^Teal$/ }).first();
    if (await teal.count()) {
        await teal.click();
    } else {
        await dialog
            .getByText("Teal", { exact: true })
            .first()
            .click()
            .catch(() => console.warn("    ! Teal theme preset not found"));
    }
    await page.waitForTimeout(CHART_SETTLE);
    record("feature-theme-dialog", await shoot(page, "feature-theme-dialog"));
    const cancel = dialog.getByRole("button", { name: /^Cancel$/ }).first();
    if (await cancel.count()) {
        await cancel.click();
    } else {
        await page.keyboard.press("Escape");
    }
    await page.waitForTimeout(FORM_SETTLE);
}

async function captureSharedTable(page, record) {
    await nav(page, "/dashboard", CHART_SETTLE);
    await selectSiteByDomain(page);
    await selectRangePreset(page, "30d");
    const card = page.locator("app-traffic-records-card").first();
    await card.waitFor({ state: "visible", timeout: 10_000 });
    await card.evaluate((element) => element.scrollIntoView({ block: "center", behavior: "instant" }));
    await page.waitForTimeout(TABLE_SETTLE);
    const box = await card.boundingBox();
    const pad = 16;
    const clip = box ? { x: Math.max(0, box.x - pad), y: Math.max(0, box.y - pad), width: box.width + pad * 2, height: Math.min(box.height + pad * 2, DESKTOP_VIEWPORT.height) } : undefined;
    record("feature-shared-table", await shoot(page, "feature-shared-table", { clip }));
}

/** A live verifier status for the demo site, so the tracking screenshots show a working install. */
function demoTrackingStatus(siteId) {
    const minutesAgo = (minutes) => new Date(Date.now() - minutes * 60_000).toISOString();
    return {
        site_id: siteId,
        tenant_id: "demo",
        status: "live",
        first_hit_at: minutesAgo(60 * 24 * 30),
        last_hit_at: minutesAgo(2),
        last_event_at: minutesAgo(6),
        last_hostname: DEMO_SITE_DOMAIN,
        last_event_name: "newsletter_signup",
        last_automatic_event_at: minutesAgo(9),
        last_automatic_event_name: "outbound_click",
        tracker_source: "hk.js",
        configured_domain: DEMO_SITE_DOMAIN,
        updated_at: minutesAgo(1)
    };
}

async function captureSiteSettings(page, record) {
    const sites = await page.evaluate(async () => (await (await fetch("/api/sites")).json()) ?? []);
    const siteId = sites.find((site) => site.domain === DEMO_SITE_DOMAIN)?.id;
    if (!siteId) {
        console.warn(`    ! ${DEMO_SITE_DOMAIN} not found, skipping site settings screenshots`);
        return;
    }
    const pattern = `**/api/sites/${siteId}/tracking/status`;
    const handler = (route) => route.fulfill({ json: demoTrackingStatus(siteId) });
    await page.route(pattern, handler);
    try {
        await nav(page, `/sites/${siteId}/settings/tracking`, FORM_SETTLE);
        await page
            .getByText(/live tracking verifier/i)
            .first()
            .waitFor({ state: "visible", timeout: 8_000 });
        const details = page.getByRole("button", { name: /details/i }).first();
        if (await details.count()) {
            await details.click();
            await page.waitForTimeout(FORM_SETTLE);
        }
        record("feature-tracking-verifier", await shoot(page, "feature-tracking-verifier"));

        const advanced = page.getByText(/advanced options/i).first();
        if (await advanced.count()) {
            await advanced.click();
            await page.waitForTimeout(FORM_SETTLE);
            await advanced.evaluate((element) => element.scrollIntoView({ block: "start", behavior: "instant" }));
            await page.waitForTimeout(400);
        }
        record("feature-site-tracking", await shoot(page, "feature-site-tracking"));
    } catch (error) {
        console.warn(`    ! Site tracking screenshots unavailable, continuing: ${error.message}`);
    } finally {
        await page.unroute(pattern, handler);
    }

    try {
        await nav(page, `/sites/${siteId}/settings/access`, FORM_SETTLE);
        await page.getByRole("heading", { name: /transfer site/i }).waitFor({ state: "visible", timeout: 8_000 });
        record("feature-site-transfer", await shoot(page, "feature-site-transfer"));
    } catch (error) {
        console.warn(`    ! Site transfer screenshot unavailable, continuing: ${error.message}`);
    }
}

async function openTeamSwitcher(page) {
    const trigger = page.locator('[data-testid="team-switcher-trigger"]:visible').first();
    if (!(await trigger.count())) return false;
    try {
        await trigger.click({ timeout: 2_000 });
        await page.waitForTimeout(FORM_SETTLE);
        return true;
    } catch (err) {
        console.warn(`    ! Team switcher trigger could not be opened: ${err.message}`);
        return false;
    }
}

async function openCreateTeamDialog(page) {
    const trigger = page.locator('[data-testid="team-switcher-add"]:visible').first();
    if (!(await trigger.count())) return false;
    try {
        await trigger.click({ timeout: 2_000 });
        await page.waitForSelector(".p-dialog", { state: "visible", timeout: 8_000 });
        await page.waitForTimeout(FORM_SETTLE);
        return true;
    } catch (err) {
        console.warn(`    ! Create team dialog could not be opened: ${err.message}`);
        return false;
    }
}

async function captureTeamCustomDomains(page, record) {
    if (!(await clickTab(page, "custom domains", TABLE_SETTLE))) {
        console.warn("    ! Custom domains tab not found, skipping custom domain screenshots");
        return;
    }

    const section = page.locator("app-team-tracking-domains");
    const hasDomains = await section.locator("tbody tr").count();
    if (!hasDomains) {
        // Create one domain through the add dialog so the table has content;
        // a successful create opens the DNS setup dialog on its own.
        await section
            .locator("header")
            .getByRole("button", { name: /add domain/i })
            .click();
        const addDialog = page.getByRole("dialog", { name: /add domain/i });
        await addDialog.getByPlaceholder("analytics.example.com").fill(`track.${DEMO_SITE_DOMAIN}`);
        await addDialog.getByRole("button", { name: /add domain/i }).click();
    } else {
        const rowActions = section.locator("app-table-row-actions button").first();
        if (await rowActions.count()) {
            await rowActions.click();
            await page.getByRole("menuitem", { name: /dns setup/i }).click();
        }
    }

    const setupDialog = page.getByRole("dialog", { name: /dns setup/i });
    await setupDialog.waitFor({ state: "visible", timeout: 8_000 }).catch(() => {});
    if (await setupDialog.count()) {
        await page.waitForTimeout(FORM_SETTLE);
        record("feature-custom-domain-setup", await shoot(page, "feature-custom-domain-setup"));
        await setupDialog.getByRole("button", { name: /cancel/i }).click();
        await page.waitForTimeout(300);
    }

    await page.waitForTimeout(TABLE_SETTLE);
    record("admin-team-custom-domains", await shoot(page, "admin-team-custom-domains"));
}

// Provider-dependent states use explicit synthetic API fixtures, as in seeded e2e.
async function captureAdditionalScreenshots(page, record) {
    const attempt = async (slug, capture) => {
        try {
            await capture();
        } catch (error) {
            record(slug, { ok: false, error: error.message });
            await page.keyboard.press("Escape").catch(() => {});
        }
    };
    const route = (slug, path) => attempt(slug, () => captureRoute(page, record, slug, path, CHART_SETTLE));
    await route("dashboard-sites-overview", "/overview");
    await route("mcp", "/integration/assistants");
    await attempt("security-sso-team-configuration", async () => {
        await nav(page, "/admin/team/sso");
        await page.locator("#sso-issuer-url").fill("https://id.example.com");
        await page.locator("#sso-client-id").fill("hitkeep-demo");
        await page.locator("#sso-allowed-domains").fill(DEMO_SITE_DOMAIN);
        record("security-sso-team-configuration", await shoot(page, "security-sso-team-configuration", { fullPage: true }));
    });
    await attempt("feature-share-dashboard", async () => {
        const sites = await (await page.request.get(`${BASE_URL}/api/sites`)).json();
        const site = sites.find((site) => site.domain === DEMO_SITE_DOMAIN);
        await captureRoute(page, record, "feature-share-dashboard", `/sites/${site.id}/settings/share`);
    });
    await route("team-traffic-exclusions-inherited", "/admin/team/exclusions");
    await route("admin-system-status", "/admin/status");
    for (const [slug, label] of [
        ["admin-system-operations", "operations"],
        ["admin-system-audit", "audit"]
    ]) {
        await attempt(slug, async () => {
            await nav(page, "/admin/status");
            if (!(await clickTab(page, label))) throw new Error(`Missing ${label} tab`);
            record(slug, await shoot(page, slug));
        });
    }
    await attempt("global_filters", async () => {
        await nav(page, "/admin/system");
        if (!(await clickTab(page, "global filters"))) throw new Error("Missing global filters tab");
        record("global_filters", await shoot(page, "global_filters"));
    });
    await page.setViewportSize(MOBILE_VIEWPORT);
    await route("feature-email-reports-mobile", "/settings/reports");
    await route("release-2-5-1-api-clients-mobile", "/integration/api-clients");
    await page.setViewportSize(DESKTOP_VIEWPORT);
    await route("release-2-5-1-api-clients-card", "/integration/api-clients");
    await attempt("release-2-5-1-api-client-grants-dialog", async () => {
        await page.locator("tbody tr").first().getByRole("button", { name: "More actions" }).click();
        await page.getByRole("menuitem", { name: "Edit", exact: true }).click();
        await page.getByRole("dialog").waitFor({ state: "visible" });
        record("release-2-5-1-api-client-grants-dialog", await shoot(page, "release-2-5-1-api-client-grants-dialog"));
        await page.keyboard.press("Escape");
    });
    await route("release-2-5-1-team-api-clients", "/admin/team/api-clients");
    await attempt("release-2-5-1-opportunities-dark", async () => {
        await nav(page, "/opportunities", CHART_SETTLE);
        await page.getByRole("button", { name: "Open user menu", exact: true }).click();
        await page.getByRole("menuitem", { name: "Switch to dark mode", exact: true }).click();
        await page.waitForFunction(() => document.documentElement.classList.contains("p-dark"));
        await page.waitForTimeout(CHART_SETTLE);
        record("release-2-5-1-opportunities-dark", await shoot(page, "release-2-5-1-opportunities-dark"));
        await page.getByRole("button", { name: "Open user menu", exact: true }).click();
        await page.getByRole("menuitem", { name: "Switch to light mode", exact: true }).click();
        await page.waitForFunction(() => !document.documentElement.classList.contains("p-dark"));
    });
    await captureOAuthScreenshots(page, record, attempt);
    await captureQRScreenshots(page, record, attempt);
    await captureWebhookScreenshots(page, record, attempt);
    await captureImportScreenshots(page, record, attempt);
    await attempt("feature-ask-ai-budget-selfhost", async () => {
        const pattern = "**/api/sites/*/ask-ai/status";
        const exhausted = { ...DEMO_ASK_AI_STATUS, available: false, status: "budget_exhausted", budget_exhausted: true };
        const handler = (route) => route.fulfill({ json: exhausted });
        await page.route(pattern, handler);
        try {
            await openAskAIDrawer(page);
            record("feature-ask-ai-budget-selfhost", await shoot(page, "feature-ask-ai-budget-selfhost"));
            await page.setViewportSize(MOBILE_VIEWPORT);
            await openAskAIDrawer(page);
            record("feature-ask-ai-budget-selfhost-mobile", await shoot(page, "feature-ask-ai-budget-selfhost-mobile"));
        } finally {
            await page.setViewportSize(DESKTOP_VIEWPORT);
            await page.unroute(pattern, handler);
        }
    });
    await attempt("session-expiry-warning", async () => {
        const pattern = "**/api/user/bootstrap";
        const handler = async (route) => {
            const response = await route.fetch();
            const body = await response.json();
            body.session = { ...body.session, expires_at: new Date(Date.now() + 90_000).toISOString(), remembered: false, warning_seconds: 120 };
            await route.fulfill({ response, json: body });
        };
        await page.route(pattern, handler);
        try {
            await nav(page, "/dashboard", CHART_SETTLE);
            await page.getByRole("alertdialog", { name: "Your session is about to expire", exact: true }).waitFor({ state: "visible" });
            record("session-expiry-warning", await shoot(page, "session-expiry-warning"));
        } finally {
            await page.unroute(pattern, handler);
        }
    });
    const anonymous = await page.context().browser().newContext({ viewport: DESKTOP_VIEWPORT, deviceScaleFactor: SCALE, locale: "en-US" });
    const loginPage = await anonymous.newPage();
    try {
        await attempt("social-sign-in-providers", () => captureRoute(loginPage, record, "social-sign-in-providers", "/login", FORM_SETTLE));
        await loginPage.route("**/api/auth/sso", (route) => route.fulfill({ json: { enabled: true } }));
        await attempt("security-sso-login", () => captureRoute(loginPage, record, "security-sso-login", "/login?method=sso&email=alex%40acme-analytics.io", FORM_SETTLE));
    } finally {
        await anonymous.close();
    }
}

async function captureOAuthScreenshots(page, record, attempt) {
    const client = { client_id: "hk_reporting_client", client_name: "Reporting application", client_uri: "https://reports.example.com", redirect_uris: ["https://reports.example.com/oauth/callback"] };
    const grant = {
        id: "reporting-grant",
        client: { id: client.client_id, name: client.client_name },
        team: { id: "demo-team", name: "Acme Analytics" },
        sites: [{ id: "demo-site", domain: DEMO_SITE_DOMAIN }],
        scopes: ["analytics:read"],
        resource: "https://analytics.example.com/mcp",
        created_at: "2026-10-01T00:00:00Z",
        expires_at: "2027-10-01T00:00:00Z",
        last_used_at: null
    };
    await page.route("**/api/user/teams/*/oauth-clients", (route) => route.fulfill({ json: [client] }));
    await page.route("**/api/user/oauth-grants", (route) => route.fulfill({ json: [grant] }));
    await attempt("oauth-clients", () => captureRoute(page, record, "oauth-clients", "/integration/oauth-clients"));
    await attempt("oauth-client-details", async () => {
        await page.locator("tbody tr").filter({ hasText: client.client_name }).getByText(client.client_name, { exact: true }).click();
        await page.getByRole("dialog", { name: client.client_name, exact: true }).waitFor({ state: "visible" });
        record("oauth-client-details", await shoot(page, "oauth-client-details"));
        await page.keyboard.press("Escape");
    });
    await attempt("oauth-client-registration", async () => {
        await page.getByRole("button", { name: "Register client", exact: true }).click();
        await page.locator("#oauth-client-name").fill("Acme reporting");
        await page.locator("#oauth-client-uri").fill("https://reports.example.com");
        await page.locator("#oauth-client-redirects").fill("https://reports.example.com/oauth/callback");
        record("oauth-client-registration", await shoot(page, "oauth-client-registration"));
        await page.keyboard.press("Escape");
    });
    await attempt("oauth-connected-applications", () => captureRoute(page, record, "oauth-connected-applications", "/settings/connected-applications"));
    await attempt("oauth-application-details", async () => {
        await page.locator("tbody tr").filter({ hasText: client.client_name }).getByText(client.client_name, { exact: true }).click();
        await page.getByRole("dialog", { name: client.client_name, exact: true }).waitFor({ state: "visible" });
        record("oauth-application-details", await shoot(page, "oauth-application-details"));
        await page.keyboard.press("Escape");
    });
    await attempt("wordpress-plugin-oauth-consent", async () => {
        await page.route("**/api/oauth/consent", (route) =>
            route.fulfill({
                json: {
                    client: { id: "wordpress-demo", name: "HitKeep for WordPress", callback_origin: "https://wordpress.example.com" },
                    resource: "https://analytics.example.com/api",
                    scopes: ["sites:read", "tracking_domains:read", "ingest:write"],
                    expires_at: new Date(Date.now() + 600_000).toISOString(),
                    csrf_token: "screenshot-fixture-only",
                    teams: [{ id: "demo-team", name: "Acme Analytics", sites: [{ id: "demo-site", domain: DEMO_SITE_DOMAIN }] }]
                }
            })
        );
        await nav(page, "/oauth/consent");
        await page.getByRole("heading", { name: "Authorize an application", exact: true }).waitFor({ state: "visible" });
        record("wordpress-plugin-oauth-consent", await shoot(page, "wordpress-plugin-oauth-consent"));
    });
}

async function captureQRScreenshots(page, record, attempt) {
    await attempt("qr-campaigns-list", () => captureRoute(page, record, "qr-campaigns-list", "/utm/qr-codes"));
    await attempt("qr-campaigns-analytics", async () => {
        await page.getByRole("link", { name: "Conference booth poster", exact: true }).click();
        await page.getByRole("heading", { name: "QR opens", exact: true }).waitFor({ state: "visible" });
        await selectRangePreset(page, "30d");
        await page
            .locator("app-kpi-card")
            .first()
            .evaluate((el) => el.scrollIntoView({ block: "start", behavior: "instant" }));
        await page.waitForTimeout(CHART_SETTLE);
        record("qr-campaigns-analytics", await shoot(page, "qr-campaigns-analytics"));
        await page.locator(".qr-codes-detail-actions button").filter({ hasText: "Create QR share link" }).click();
        await page.getByRole("dialog", { name: "QR share links", exact: true }).waitFor({ state: "visible" });
        record("qr-campaigns-share-links", await shoot(page, "qr-campaigns-share-links"));
        await page.keyboard.press("Escape");
        await page
            .locator(".qr-codes-detail-actions button")
            .filter({ hasText: /^Edit$/ })
            .click();
        await page.waitForTimeout(FORM_SETTLE);
        record("qr-campaigns-editor", await shoot(page, "qr-campaigns-editor"));
        await page.keyboard.press("Escape");
    });
}

async function captureWebhookScreenshots(page, record, attempt) {
    await attempt("integration-webhooks", () => captureRoute(page, record, "integration-webhooks", "/integration/webhooks"));
    await attempt("integration-webhook-form", async () => {
        await page.getByTestId("create-webhook").click();
        await page.getByLabel("Name", { exact: true }).fill("Release notifications");
        await page.getByLabel("Destination URL").fill("https://hooks.example.com/hitkeep");
        record("integration-webhook-form", await shoot(page, "integration-webhook-form"));
        await page.locator(".event-options").scrollIntoViewIfNeeded();
        record("integration-webhooks-events", await shoot(page, "integration-webhooks-events"));
        await page.keyboard.press("Escape");
    });
    await attempt("integration-webhook-deliveries", async () => {
        await nav(page, "/integration/webhooks");
        await page.locator("tbody tr").first().getByRole("button", { name: "More actions" }).click();
        await page.getByRole("menuitem", { name: /deliveries/i }).click();
        await page.getByRole("dialog").waitFor({ state: "visible" });
        record("integration-webhook-deliveries", await shoot(page, "integration-webhook-deliveries"));
        await page.keyboard.press("Escape");
    });
}

async function captureImportScreenshots(page, record, attempt) {
    await attempt("imports-plausible-provider", () => captureRoute(page, record, "imports-plausible-provider", "/import-export/import"));
    await attempt("imports-simple-analytics-upload", async () => {
        await page.getByRole("button", { name: /select simple analytics importer/i }).click();
        record("imports-simple-analytics-upload", await shoot(page, "imports-simple-analytics-upload"));
    });
    await attempt("imports-plausible-zip-review", async () => {
        const previousRange = await page.evaluate(() => localStorage.getItem("hitkeep.reportRange"));
        const site = await page.evaluate(async (domain) => {
            const response = await fetch("/api/sites", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ domain }) });
            if (!response.ok) throw new Error(`Create synthetic import site: ${response.status}`);
            return response.json();
        }, `screenshot-import-${Date.now()}.example.test`);
        try {
            const fixtureDir = resolve("tests/fixtures/imports/plausible");
            const files = readdirSync(fixtureDir, { recursive: true }).filter((file) => file.endsWith(".csv"));
            if (files.length !== 11) throw new Error("Expected the complete Plausible import fixture");
            const zip = join(OUTPUT_DIR, "plausible-fixture.zip");
            execFileSync("zip", ["-q", zip, ...files], { cwd: fixtureDir });
            await nav(page, "/import-export/import");
            await selectSiteByDomain(page, site.domain);
            await page.getByRole("button", { name: /select plausible importer/i }).click();
            await page.locator('input[type="file"]').setInputFiles(zip);
            await page.getByRole("button", { name: /upload and validate/i }).click();
            await page.getByText("Validation manifest", { exact: true }).waitFor({ state: "visible", timeout: 30_000 });
            if (!(await page.locator(".imports-manifest-grid").innerText()).includes("16")) throw new Error("Import fixture validation did not accept 16 rows");
            record("imports-plausible-zip-review", await shoot(page, "imports-plausible-zip-review", { fullPage: true }));
            await page.getByRole("button", { name: /start import/i }).click();
            await page.getByText("Import completed.", { exact: true }).waitFor({ state: "visible", timeout: 30_000 });
            // Keep the checked-in fixture's exact dates independent of the current year.
            await page.evaluate(() => localStorage.setItem("hitkeep.reportRange", JSON.stringify({ value: "custom", customRange: ["2026-04-02T00:00:00Z", "2026-05-01T00:00:00Z"] })));
            await nav(page, "/events", CHART_SETTLE);
            await selectVisibleOptionMatching(page, "#event-name-select", /outbound_click/);
            const total = await page.evaluate(async (siteId) => {
                const rows = await (await fetch(`/api/sites/${siteId}/events/timeseries?from=2026-04-02T00%3A00%3A00Z&to=2026-05-01T00%3A00%3A00Z&event_name=outbound_click`)).json();
                return rows.reduce((sum, row) => sum + row.count, 0);
            }, site.id);
            if (total !== 3) throw new Error(`Imported fixture should contain three outbound clicks, got ${total}`);
            await page.waitForTimeout(CHART_SETTLE);
            record("imports-plausible-events-report", await shoot(page, "imports-plausible-events-report"));
        } finally {
            await page.evaluate(async (siteId) => {
                const imports = await (await fetch(`/api/sites/${siteId}/imports`)).json();
                for (const job of imports.imports || []) {
                    const response = await fetch(`/api/sites/${siteId}/imports/${job.id}`, { method: "DELETE" });
                    if (!response.ok) throw new Error(`Cleanup synthetic import: ${response.status}`);
                }
                const deleted = await fetch(`/api/sites/${siteId}`, { method: "DELETE" });
                if (!deleted.ok) throw new Error(`Cleanup synthetic import site: ${deleted.status}`);
            }, site.id);
            await page.evaluate((range) => (range === null ? localStorage.removeItem("hitkeep.reportRange") : localStorage.setItem("hitkeep.reportRange", range)), previousRange);
            await nav(page, "/dashboard");
            await selectSiteByDomain(page);
        }
    });
}

async function run() {
    console.log("\n  HitKeep Dashboard Screenshot Tool");
    console.log("  ────────────────────────────────");
    console.log(`  Instance : ${BASE_URL}`);
    console.log(`  Output   : ${OUTPUT_DIR}`);
    console.log(`  Scale    : ${SCALE}x\n`);

    mkdirSync(OUTPUT_DIR, { recursive: true });

    const browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({
        viewport: DESKTOP_VIEWPORT,
        deviceScaleFactor: SCALE,
        colorScheme: "light"
    });

    context.setDefaultTimeout(10_000);
    const page = await context.newPage();
    page.on("console", () => {});
    page.on("pageerror", () => {});
    await installAskAIDemoRoutes(page);

    const results = [];
    const record = (slug, result) => {
        results.push({ slug, ...result });
        writeFileSync(join(OUTPUT_DIR, "capture-progress.json"), JSON.stringify({ script_sha256: SCRIPT_SHA256, results }, null, 2));
        console.log(`    ${result.ok ? "✓" : "✗"} ${slug}${result.ok ? "" : ` — ${result.error}`}`);
    };

    try {
        // Targeted runs capture only their subset; the login page belongs to the full set.
        if (SCREENSHOT_TARGET === "") {
            console.log("  Pre-auth:");
            await page.goto(`${BASE_URL}/login`, { waitUntil: "domcontentloaded" });
            await page.waitForSelector('input[type="password"]', { state: "visible", timeout: 10_000 });
            await page.waitForTimeout(FORM_SETTLE);
            record("page-login", await shoot(page, "page-login"));
        }

        console.log("\n  Authenticating...");
        await login(page);
        console.log("  ✓ Logged in\n");

        if (SCREENSHOT_TARGET === "ask-ai") {
            console.log("  Ask AI:");
            await captureAskAI(page, record);
        } else if (SCREENSHOT_TARGET === "annotations") {
            console.log("  Annotations:");
            await captureAnnotations(page, record);
        } else if (SCREENSHOT_TARGET === "release") {
            console.log("  Release highlights:");
            await captureAnnotations(page, record);
            await captureReportEmailPreview(page, record);
            await captureThemeDialog(page, record);
            await captureSharedTable(page, record);
        } else {
            console.log("  Dashboard:");
            await captureRoute(page, record, "dashboard-overview", "/dashboard", CHART_SETTLE);
            const onboardingPattern = "**/api/user/onboarding";
            await page.route(onboardingPattern, (route) =>
                route.fulfill({
                    json: {
                        dismissed: false,
                        complete: false,
                        steps: [
                            { key: "create_site", complete: true },
                            { key: "verify_tracking", complete: false, site_domain: "launch-waiting.example.com" },
                            { key: "automatic_events", complete: false },
                            { key: "invite_teammate", complete: false },
                            { key: "schedule_report", complete: false }
                        ]
                    }
                })
            );
            try {
                await selectSiteByDomain(page, "launch-waiting.example.com");
                await page.locator('[aria-labelledby="dashboard-onboarding-title"]').evaluate((el) => el.scrollIntoView({ block: "start", behavior: "instant" }));
                record("feature-onboarding-checklist", await shoot(page, "feature-onboarding-checklist"));
            } finally {
                await page.unroute(onboardingPattern);
                await selectSiteByDomain(page);
            }

            if (await openTeamSwitcher(page)) {
                record("feature-team-switcher", await shoot(page, "feature-team-switcher"));
                await page.keyboard.press("Escape");
                await page.waitForTimeout(300);
            } else {
                console.warn("    ! Team switcher combobox not found, skipping switcher screenshot");
            }

            if (await openCreateTeamDialog(page)) {
                record("feature-create-team", await shoot(page, "feature-create-team"));
                await page.keyboard.press("Escape");
                await page.waitForTimeout(300);
            } else {
                console.warn("    ! Create team button not found, skipping dialog screenshot");
            }

            const shareBtn = page.getByRole("button", { name: /share dashboard/i }).first();
            if (await shareBtn.count()) {
                await shareBtn.click();
                await page.waitForSelector(".p-dialog", { state: "visible", timeout: 8_000 });
                await page.waitForTimeout(FORM_SETTLE);
                record("feature-share-dashboard", await shoot(page, "feature-share-dashboard"));
                await page.keyboard.press("Escape");
                await page.waitForTimeout(300);
            } else {
                console.warn("    ! Share dashboard button not found, skipping dialog screenshot");
            }

            await captureSiteSettings(page, record);

            console.log("\n  Analytics:");
            await captureAnnotations(page, record);
            await captureRoute(page, record, "analytics-goals", "/goals", CHART_SETTLE);
            await captureRoute(page, record, "analytics-funnels", "/funnels", CHART_SETTLE);
            await captureRoute(page, record, "analytics-ecommerce", "/ecommerce", CHART_SETTLE);

            await nav(page, "/events", CHART_SETTLE);
            await prepareEventBreakdown(page);

            await page.evaluate(() => window.scrollTo({ top: 0, behavior: "instant" }));
            record("analytics-events", await shoot(page, "analytics-events"));

            await page.evaluate(() => {
                const audience = document.querySelector("app-metric-card-group");
                if (audience) {
                    audience.scrollIntoView({ behavior: "instant", block: "start" });
                } else {
                    window.scrollTo({ top: Math.floor(window.innerHeight * 0.8), behavior: "instant" });
                }
            });
            await page.waitForTimeout(TABLE_SETTLE);
            record("analytics-events-audience", await shoot(page, "analytics-events-audience"));

            // One page now: the hero KPIs and chart at the top, then the one card grid
            // holding every breakdown, then the fetch depth card. All three shots come
            // from /ai-agents, the last two scrolled.
            await captureRoute(page, record, "analytics-ai-agents-traffic", "/ai-agents", CHART_SETTLE);

            await page.evaluate(() => {
                const target = document.querySelector('[data-testid="ai-agents-page-chips"], [data-testid="ai-agents-enrich-callout"]');
                if (target) {
                    target.scrollIntoView({ behavior: "instant", block: "start" });
                } else {
                    window.scrollTo({ top: document.documentElement.scrollHeight, behavior: "instant" });
                }
            });
            await page.waitForTimeout(TABLE_SETTLE);
            record("analytics-ai-visibility", await shoot(page, "analytics-ai-visibility"));
            const aiVisibilityClip = await prepareAIVisibilityShot(page);
            record("analytics-ai-visibility-correlation", await shoot(page, "analytics-ai-visibility-correlation", { clip: aiVisibilityClip ?? undefined }));
            await page.setViewportSize(DESKTOP_VIEWPORT);
            await captureRoute(page, record, "analytics-ai-chatbots", "/ai-chatbots", CHART_SETTLE);
            await captureWebVitals(page, record);
            await captureOpportunities(page, record, "analytics-opportunities");
            await captureRoute(page, record, "analytics-utm", "/utm", CHART_SETTLE);
            await captureRoute(page, record, "dashboard-comparison", "/dashboard", CHART_SETTLE);
            await captureSearchConsoleDrilldown(page, record, "analytics-search-console");
            await captureAskAI(page, record);

            console.log("\n  Settings:");
            await captureRoute(page, record, "settings-profile", "/settings", FORM_SETTLE);

            await clickTab(page, "^Security$");
            await page.evaluate(() => {
                const el = document.querySelector("app-settings-security");
                if (el) el.scrollIntoView({ behavior: "instant", block: "start" });
            });
            await page.waitForTimeout(400);
            record("security-2fa-setup", await shoot(page, "security-2fa-setup"));

            await captureRoute(page, record, "feature-email-reports", "/settings/reports", FORM_SETTLE);
            await captureReportEmailPreview(page, record);
            await captureThemeDialog(page, record);
            await captureSharedTable(page, record);

            console.log("\n  Integrations:");
            await captureRoute(page, record, "security-api-clients", "/integration/api-clients", TABLE_SETTLE);
            await captureRoute(page, record, "integration-api-reference", "/integration/api-reference", TABLE_SETTLE);
            await captureGoogleSearchConsoleIntegration(page, record, "integration-google-search-console");

            console.log("\n  Mobile spot checks:");
            await page.setViewportSize(MOBILE_VIEWPORT);
            await captureGoogleSearchConsoleIntegration(page, record, "integration-google-search-console-mobile");
            await captureSearchConsoleDrilldown(page, record, "analytics-search-console-mobile");
            await page.setViewportSize(DESKTOP_VIEWPORT);

            console.log("\n  Admin:");
            await nav(page, "/admin/system", TABLE_SETTLE);
            let adminSitesCaptured = false;
            if (await clickTab(page, "sites")) {
                record("admin-sites-list", await shoot(page, "admin-sites-list"));
                adminSitesCaptured = true;
            }
            if (!adminSitesCaptured) {
                const sitesTabFallback = page.getByText(/^Sites$/).first();
                if (await sitesTabFallback.count()) {
                    await sitesTabFallback.click();
                    await page.waitForTimeout(TABLE_SETTLE);
                    record("admin-sites-list", await shoot(page, "admin-sites-list"));
                } else {
                    console.warn("    ! Sites tab not found, skipping admin sites screenshot");
                }
            }
            await nav(page, "/admin/status", TABLE_SETTLE);
            if (await clickTab(page, "activation", TABLE_SETTLE)) {
                await page
                    .getByText(/user activation/i)
                    .first()
                    .waitFor({ state: "visible", timeout: 8_000 });
                record("admin-system-activation", await shoot(page, "admin-system-activation"));
            } else {
                console.warn("    ! Activation tab not found, skipping admin activation screenshot");
            }
            await captureRoute(page, record, "admin-team-overview", "/admin/team", TABLE_SETTLE);

            if (await clickTab(page, "members")) {
                record("admin-team-members", await shoot(page, "admin-team-members"));
            }
            if (await clickTab(page, "api clients", TABLE_SETTLE)) {
                record("admin-team-api-clients", await shoot(page, "admin-team-api-clients"));
            }
            await captureTeamCustomDomains(page, record);
            if (await clickTab(page, "branding", FORM_SETTLE)) {
                record("admin-team-settings", await shoot(page, "admin-team-settings"));
            }
            if (await clickTab(page, "activity", FORM_SETTLE)) {
                record("admin-team-audit", await shoot(page, "admin-team-audit"));
            }

            console.log("\n  Tools:");
            await captureRoute(page, record, "tools-utm-builder", "/utm/builder", FORM_SETTLE);
            await captureAdditionalScreenshots(page, record);
        }
    } catch (error) {
        // Keep the page state that broke the run next to the captures.
        await page.screenshot({ path: join(OUTPUT_DIR, "_failure.png"), fullPage: true }).catch(() => {});
        record("capture-run", { ok: false, error: error.message, stack: error.stack, route: new URL(page.url()).pathname });
    } finally {
        await browser.close();
    }

    if (SCREENSHOT_TARGET === "") {
        for (const slug of EXPECTED_SCREENSHOTS) {
            if (!results.some((result) => result.slug === slug && result.ok)) {
                results.push({ slug, ok: false, error: "Expected screenshot was not captured" });
            }
        }
    }
    const ok = results.filter((r) => r.ok);
    const failed = results.filter((r) => !r.ok);
    writeFileSync(
        join(OUTPUT_DIR, "capture-manifest.json"),
        JSON.stringify(
            {
                captured_at: new Date().toISOString(),
                source_revision: execFileSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" }).trim(),
                script_sha256: SCRIPT_SHA256,
                target: SCREENSHOT_TARGET,
                fixtures: [
                    "seeded synthetic analytics",
                    "Ask AI deterministic SSE and budget state",
                    "annotations",
                    "tracking verifier",
                    "incomplete onboarding",
                    "OAuth management and WordPress consent",
                    "Assistants HTTPS endpoint",
                    "SSO availability",
                    "session expiry",
                    "checked-in Plausible CSV fixture"
                ],
                results
            },
            null,
            2
        )
    );

    console.log(`\n  ✓ ${ok.length} screenshot(s) saved to:`);
    console.log(`    ${OUTPUT_DIR}`);

    if (failed.length) {
        console.log(`\n  ✗ ${failed.length} failed:`);
        failed.forEach((r) => console.log(`    - ${r.slug}: ${r.error}`));
    }

    if (failed.length) process.exit(1);
}

function fail(error) {
    mkdirSync(OUTPUT_DIR, { recursive: true });
    writeFileSync(join(OUTPUT_DIR, "_failure.json"), JSON.stringify({ message: error?.message ?? String(error), stack: error?.stack, script_sha256: SCRIPT_SHA256 }, null, 2));
    console.error("Fatal:", error?.message ?? String(error));
    process.exit(1);
}

process.on("unhandledRejection", fail);
process.on("uncaughtException", fail);
run().catch(fail);
