const { test, expect } = require("playwright/test");
const { login } = require("./support/auth");

const client = {
    client_id: "hk_reporting_client",
    client_name: "Reporting application",
    client_uri: "https://reports.example.com",
    redirect_uris: ["https://reports.example.com/oauth/callback"]
};
const grant = {
    id: "reporting-grant",
    client: { id: client.client_id, name: client.client_name },
    team: { id: "reporting-team", name: "Reporting team" },
    sites: [{ id: "reporting-site", name: "Reporting site", domain: "reporting.example.com" }],
    scopes: ["analytics:read"],
    resource: "https://analytics.example.com/mcp",
    created_at: "2026-01-01T00:00:00Z",
    expires_at: "2027-01-01T00:00:00Z",
    last_used_at: null
};

for (const viewport of [
    { name: "desktop", width: 1440, height: 1000 },
    { name: "mobile", width: 390, height: 844 }
]) {
    test(`OAuth management opens details in shared dialogs on ${viewport.name}`, async ({ page }, testInfo) => {
        await page.setViewportSize({ width: viewport.width, height: viewport.height });
        // The default seeded server has no HTTPS OAuth issuer; fixtures isolate the management UI.
        await page.route("**/api/user/teams/*/oauth-clients", (route) => route.fulfill({ json: [client] }));
        await page.route("**/api/user/oauth-grants", (route) => route.fulfill({ json: [grant] }));
        await login(page, "/integration/oauth-clients");

        const darkMode = page.getByRole("button", { name: "Switch to dark mode", exact: true });
        if (viewport.name === "mobile" && (await darkMode.count())) await darkMode.click();
        const clients = page.locator("app-table");
        const clientRow = clients.locator("tbody tr", { hasText: client.client_name });
        await expect(clientRow).toBeVisible();
        await expect(clients).not.toContainText(client.redirect_uris[0]);
        await clientRow.getByText(client.client_name, { exact: true }).click();
        const details = page.getByRole("dialog", { name: client.client_name, exact: true });
        await expect(details).toBeVisible();
        await expect(details).toContainText(client.client_id);
        await expect(details).toContainText(client.redirect_uris[0]);
        await expectWithinViewport(details, viewport.width);
        await details.screenshot({ path: testInfo.outputPath("oauth-client-details.png") });
        await details.getByRole("button", { name: "Edit", exact: true }).click();
        await expect(details).toBeHidden();
        const editor = page.getByRole("dialog", { name: "Edit client", exact: true });
        await expect(editor).toBeVisible();
        await expect(editor.getByLabel("Name", { exact: true })).toHaveValue(client.client_name);
        await page.keyboard.press("Escape");
        await expect(editor).toBeHidden();

        await page.goto("/settings/connected-applications");
        const grants = page.locator("app-table");
        const grantRow = grants.locator("tbody tr", { hasText: client.client_name });
        await expect(grantRow).toBeVisible();
        await expect(grants).not.toContainText(grant.resource);
        await grantRow.getByText(client.client_name, { exact: true }).click();
        await expect(details).toBeVisible();
        await expect(details).toContainText(grant.resource);
        await expect(details).toContainText("analytics:read");
        await expect(details).toContainText("reporting.example.com");
        await expectWithinViewport(details, viewport.width);
        await details.screenshot({ path: testInfo.outputPath("oauth-grant-details.png") });
        await details.getByRole("button", { name: "Close", exact: true }).last().click();
        await expect(details).toBeHidden();
        await expect(grants).not.toContainText(grant.resource);
    });
}

async function expectWithinViewport(dialog, width) {
    const bounds = await dialog.boundingBox();
    expect(bounds).not.toBeNull();
    expect(bounds.x).toBeGreaterThanOrEqual(0);
    expect(bounds.x + bounds.width).toBeLessThanOrEqual(width + 1);
}
