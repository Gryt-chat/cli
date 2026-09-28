import { chromium } from "playwright";

export const APP = process.env.GRYT_E2E_APP || "https://app.gryt.chat";

export async function launch() {
  return chromium.launch({
    args: ["--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream"],
  });
}

export async function newGuest(browser, label, log) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  const page = await context.newPage();
  const errors = [];
  page.on("console", (m) => {
    if (m.type() === "error") errors.push(m.text().slice(0, 300));
  });
  page.on("pageerror", (e) => errors.push("pageerror " + String(e).slice(0, 300)));
  await page.goto(APP + "/");
  const welcome = page.getByRole("button", { name: "I’ll look myself" });
  await welcome.click({ timeout: 30_000 }).catch(() => log?.(`${label}: no welcome dialog`));
  return { context, page, label, errors };
}

export async function joinByAddress(guest, host) {
  const { page } = guest;
  await page.getByRole("button", { name: "Add a server" }).first().click();
  const dialog = page.getByRole("dialog", { name: "Join a server" });
  await dialog.getByLabel("Invite or server address").fill(host);
  await dialog.getByText("No account needed").waitFor({ timeout: 20_000 });
  await dialog.getByRole("button", { name: "Join", exact: true }).click();
  await dialog.waitFor({ state: "hidden", timeout: 30_000 });
  await composer(page).waitFor({ timeout: 30_000 });
}

export function composer(page, channel = "General") {
  return page.locator(`[role="textbox"][aria-placeholder="Message #${channel}"]`);
}

export const CONFIRMED_ROW = '[data-message-id]:not([data-message-id^="pending-"])';

export async function agreeIfAsked(page) {
  const agree = page.getByRole("button", { name: /^(I agree|Agree|Accept)/ });
  if (await agree.count()) await agree.first().click().catch(() => {});
}

export async function sendMessage(page, text) {
  const box = composer(page);
  await box.click();
  await page.keyboard.insertText(text);
  await box.press("Enter");
  await agreeIfAsked(page);
  await page.locator(CONFIRMED_ROW).filter({ hasText: text }).waitFor({ timeout: 20_000 });
}

export async function attachAndSend(page, files) {
  const chooser = page.waitForEvent("filechooser");
  await page.getByRole("button", { name: "Attach file" }).click();
  await (await chooser).setFiles(files);
  await composer(page).press("Enter");
  await agreeIfAsked(page);
  await page.getByRole("button", { name: "Remove file" }).first().waitFor({ state: "detached", timeout: 30_000 });
}

export async function poll(fn, ok, timeoutMs, stepMs = 1000) {
  const end = Date.now() + timeoutMs;
  let last;
  while (Date.now() < end) {
    last = await fn();
    if (ok(last)) return last;
    await new Promise((r) => setTimeout(r, stepMs));
  }
  throw new Error(`timed out; last value ${JSON.stringify(last)}`);
}
