/**
 * e2e/global-setup.ts — Playwright global setup.
 *
 * Runs once before all test files:
 *   1. Seeds the database with known test skins (via cmd/e2e-seed).
 *   2. Obtains a Pocket-ID one-time login token for the admin user.
 *   3. Performs the full OIDC login flow in a throw-away browser.
 *   4. Saves the authenticated browser state to e2e/.auth/admin.json.
 *
 * Admin tests load this state via `storageState` so they start
 * already logged in.
 */

import { chromium, type FullConfig } from "@playwright/test";
import { execSync } from "child_process";
import fs from "fs";
import path from "path";
import https from "https";

const AUTH_DIR = path.join(__dirname, ".auth");
const ADMIN_STATE = path.join(AUTH_DIR, "admin.json");

/** Reads the Pocket-ID config from docker/dev.env. */
function loadEnv(): Record<string, string> {
  const envPath = path.resolve(__dirname, "..", "docker", "dev.env");
  const text = fs.readFileSync(envPath, "utf-8");
  const env: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    const idx = trimmed.indexOf("=");
    if (idx < 0) continue;
    env[trimmed.slice(0, idx)] = trimmed.slice(idx + 1).replace(/^["']|["']$/g, "");
  }
  return env;
}

/** Calls the Pocket-ID admin API to get a one-time access token. */
async function getOneTimeToken(env: Record<string, string>): Promise<string> {
  // First, list users to find the admin user ID.
  const apiKey = env.POCKET_ID_STATIC_API_KEY;
  const base = env.OIDC_ISSUER_URL; // https://localhost:1411

  const fetchJSON = (urlPath: string, method = "GET", body?: string): Promise<any> =>
    new Promise((resolve, reject) => {
      const url = new URL(urlPath, base);
      const req = https.request(
        url,
        {
          method,
          headers: {
            "X-API-Key": apiKey,
            "Content-Type": "application/json",
          },
          rejectUnauthorized: false, // self-signed cert
        },
        (res) => {
          let data = "";
          res.on("data", (c) => (data += c));
          res.on("end", () => {
            if (res.statusCode && res.statusCode >= 400) {
              reject(new Error(`Pocket-ID ${method} ${urlPath}: HTTP ${res.statusCode}: ${data}`));
            } else {
              resolve(JSON.parse(data));
            }
          });
        },
      );
      req.on("error", reject);
      if (body) req.write(body);
      req.end();
    });

  // List users — find the non-static-API user.
  const usersResp = await fetchJSON("/api/users");
  const users: Array<{ id: string }> = usersResp.data ?? usersResp;
  const adminUser = users.find((u) => u.id !== "00000000-0000-0000-0000-000000000000");
  if (!adminUser) throw new Error("No admin user found in Pocket-ID");

  // Create one-time access token.
  const tokenResp = await fetchJSON(`/api/users/${adminUser.id}/one-time-access-token`, "POST", "{}");
  return tokenResp.token;
}

export default async function globalSetup(_config: FullConfig) {
  const baseURL = _config.projects[0]?.use?.baseURL ?? "http://localhost:8080";

  // ── 1. Seed test data ──────────────────────────────────────────────────
  console.log("▸ Seeding test data…");
  try {
    execSync(`go run ./cmd/e2e-seed -addr ${baseURL}`, {
      stdio: "inherit",
      cwd: path.resolve(__dirname, ".."),
    });
  } catch {
    console.warn("⚠ Seed command failed (data may already exist).");
  }

  // ── 2. Perform admin OIDC login ────────────────────────────────────────
  console.log("▸ Performing admin OIDC login…");
  const env = loadEnv();
  const otToken = await getOneTimeToken(env);

  fs.mkdirSync(AUTH_DIR, { recursive: true });

  const browser = await chromium.launch();
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  const page = await context.newPage();

  // 2a. Visit one-time login URL → authenticates on Pocket-ID.
  const otURL = `${env.OIDC_ISSUER_URL}/lc/${otToken}`;
  await page.goto(otURL, { waitUntil: "networkidle", timeout: 20_000 });

  // 2b. Initiate OIDC login on the asset-service.
  //     The authorize page may auto-redirect or show a consent button.
  await page.goto(`${baseURL}/auth/login`, { timeout: 30_000 });

  // Wait until we land back on the asset-service (OIDC redirect chain completes).
  await page.waitForURL(`${baseURL}/**`, { timeout: 30_000 });

  // 2c. Save authenticated storage state for admin tests.
  await context.storageState({ path: ADMIN_STATE });
  console.log(`  ✓ Admin auth state saved to ${ADMIN_STATE}`);

  await browser.close();
}
