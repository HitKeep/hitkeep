const { test, expect } = require("playwright/test");

test.use({ locale: "de-DE" });

for (const [selector, lang, heading] of [
    ["en", "en", "Get started with HitKeep Pro"],
    ["fr", "fr", "Commencer avec HitKeep Pro"],
    ["unknown", "de-DE", "Starte mit HitKeep Pro"],
    ["", "de-DE", "Starte mit HitKeep Pro"]
]) {
    test(`signed-out signup in a German browser with ${selector || "no"} language selector`, async ({ page }) => {
        await page.route("**/api/status", (route) => route.fulfill({ json: { needs_setup: false, cloud: { hosted: true, signup_enabled: true, jurisdiction: "EU" } } }));
        await page.addInitScript(() => {
            new MutationObserver((_changes, observer) => {
                const title = document.querySelector("#signup-title")?.textContent.trim();
                if (title) {
                    window.firstSignupTitle = title;
                    observer.disconnect();
                }
            }).observe(document, { childList: true, subtree: true, characterData: true });
        });
        const query = new URLSearchParams({ plan: "pro", billing: "annual" });
        if (selector) query.set("lang", selector);
        await page.goto(`/signup?${query}`);
        await expect(page.getByRole("heading", { level: 1 })).toHaveText(heading);
        expect(await page.evaluate(() => window.firstSignupTitle)).toBe(heading);
        await expect(page.locator("html")).toHaveAttribute("lang", lang);

        await page.locator('a[href^="/login"]').click();
        await page.waitForURL("**/login?**");
        await page.reload();
        await expect(page.locator("html")).toHaveAttribute("lang", "de-DE");
        const signup = page.locator('a[href^="/signup"]');
        const signupHref = await signup.getAttribute("href");
        expect(new URL(signupHref, page.url()).searchParams.get("lang")).toBe(lang === "de-DE" ? null : lang);
        await signup.click();
        await expect(page.getByRole("heading", { level: 1 })).toHaveText(heading);
    });
}
