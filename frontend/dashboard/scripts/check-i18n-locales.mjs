import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { DASHBOARD_LANGUAGE_CODES } from "../src/app/core/i18n/supported-locales.ts";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(scriptDir, "..");
const localeDir = path.join(root, "public", "i18n");
const requiredKeys = [
    {
        path: "admin.system.health.ip2locationAttribution",
        includes: ["HitKeep", "IP2Location LITE"]
    }
];
const forbiddenPaths = ["password.webhooks"];

if (!fs.existsSync(localeDir)) {
    throw new Error(`public/i18n not found at ${localeDir}.`);
}

const localeFiles = fs
    .readdirSync(localeDir)
    .filter((file) => file.endsWith(".json"))
    .sort((a, b) => a.localeCompare(b, "en", { sensitivity: "base" }));

if (localeFiles.length === 0) {
    throw new Error("No public/i18n locale files found.");
}

const failures = [];
for (const lang of DASHBOARD_LANGUAGE_CODES) {
    if (!localeFiles.includes(`${lang}.json`)) failures.push(`Missing catalog for ${lang}`);
}

const englishPath = path.join(localeDir, "en.json");
if (!fs.existsSync(englishPath)) {
    throw new Error("English locale file is required as the namespace reference.");
}
const english = JSON.parse(fs.readFileSync(englishPath, "utf8"));

for (const file of localeFiles) {
    const fullPath = path.join(localeDir, file);
    const locale = path.basename(file, ".json");
    if (!DASHBOARD_LANGUAGE_CODES.includes(locale)) failures.push(`Unexpected catalog ${file}`);
    const data = JSON.parse(fs.readFileSync(fullPath, "utf8"));

    for (const requirement of requiredKeys) {
        const value = readPath(data, requirement.path);
        if (typeof value !== "string" || value.trim() === "") {
            failures.push(`${locale}: missing ${requirement.path}`);
            continue;
        }
        for (const requiredText of requirement.includes) {
            if (!value.includes(requiredText)) {
                failures.push(`${locale}: ${requirement.path} must include ${requiredText}`);
            }
        }
    }

    checkTranslations(english, data, locale);

    for (const forbiddenPath of forbiddenPaths) {
        if (readPath(data, forbiddenPath) !== undefined) {
            failures.push(`${locale}: misplaced namespace ${forbiddenPath}`);
        }
    }
}

if (failures.length > 0) {
    throw new Error(`Locale check failed:\n${failures.map((failure) => `- ${failure}`).join("\n")}`);
}

console.log(`Checked ${localeFiles.length} public i18n locale files.`);

function readPath(value, dottedPath) {
    return dottedPath.split(".").reduce((current, part) => {
        if (current && typeof current === "object" && part in current) {
            return current[part];
        }
        return undefined;
    }, value);
}

function checkTranslations(reference, translated, locale, prefix = "") {
    if (!translated || typeof translated !== "object" || Array.isArray(translated)) {
        failures.push(`${locale}: invalid namespace ${prefix.slice(0, -1) || "(root)"}`);
        return;
    }
    for (const [key, expected] of Object.entries(reference)) {
        const fullPath = prefix + key;
        const actual = translated[key];
        if (typeof expected === "string") {
            if (typeof actual !== "string" || !actual.trim()) {
                failures.push(`${locale}: missing or invalid ${fullPath}`);
            } else if (interpolationParams(expected).join(",") !== interpolationParams(actual).join(",")) {
                failures.push(`${locale}: ${fullPath} interpolation must match English`);
            }
        } else if (expected && typeof expected === "object" && !Array.isArray(expected)) {
            if (actual === undefined && !Object.keys(expected).length) {
                failures.push(`${locale}: missing ${fullPath}`);
            } else {
                checkTranslations(expected, actual === undefined ? {} : actual, locale, fullPath + ".");
            }
        } else {
            failures.push(`en: invalid translation shape at ${fullPath}`);
        }
    }
    for (const key of Object.keys(translated)) {
        if (!Object.hasOwn(reference, key)) {
            failures.push(`${locale}: unexpected ${prefix}${key}`);
        }
    }
}

function interpolationParams(value) {
    return [...value.matchAll(/\{\{\s*([\w.-]+)\s*\}\}/g)].map((match) => match[1]).sort();
}
