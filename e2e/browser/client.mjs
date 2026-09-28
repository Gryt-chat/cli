// Drives the released web client against a server made by the gryt CLI. Prints one
// RESULT line per check, which run-cli.sh collects. Usage:
//   node client.mjs chat <host>   a guest joins, sends a message, uploads an image
import { mkdirSync } from "node:fs";
import { attachAndSend, composer, CONFIRMED_ROW, joinByAddress, launch, newGuest, poll, sendMessage } from "./lib.mjs";

const [mode, host] = process.argv.slice(2);
const OUT = process.env.GRYT_E2E_OUT || "out";
mkdirSync(OUT, { recursive: true });
let failures = 0;

async function check(name, fn) {
  const started = Date.now();
  try {
    const detail = await fn();
    console.log(`RESULT\tPASS\t${name}\t${((Date.now() - started) / 1000).toFixed(1)}s\t${detail ?? ""}`);
    return true;
  } catch (e) {
    failures++;
    const msg = String(e?.message ?? e).split("\n").slice(0, 3).join(" | ");
    console.log(`RESULT\tFAIL\t${name}\t${((Date.now() - started) / 1000).toFixed(1)}s\t${msg}`);
    return false;
  }
}

async function snap(guest, name) {
  await guest.page.screenshot({ path: `${OUT}/${name}.png` }).catch(() => {});
}

if (mode !== "chat") {
  console.error(`unknown mode ${mode}; only "chat" exists`);
  process.exit(2);
}

const browser = await launch();
const log = (m) => console.log(m);
try {
  const guest = await newGuest(browser, "guest", log);
  const joined = await check(`join ${host} as a guest`, async () => {
    await joinByAddress(guest, host);
    return "local identity, first member";
  });
  await snap(guest, "chat-joined");
  if (!joined) throw new Error("could not join; later checks skipped");

  await check("send a message", async () => {
    await sendMessage(guest.page, `hello from the CLI e2e ${Date.now()}`);
  });

  await check("upload an image, thumbnail made", async () => {
    await attachAndSend(guest.page, ["fixtures/gradient.png"]);
    const img = guest.page.locator(`${CONFIRMED_ROW} img[alt="gradient.png"]`);
    await img.waitFor({ timeout: 30_000 });
    // The sender draws its own blob until a reload; after one the src is the server's copy.
    await guest.page.reload();
    await composer(guest.page).waitFor();
    await img.waitFor({ timeout: 30_000 });
    await poll(() => img.evaluate((i) => i.complete && i.naturalWidth), (w) => w > 0, 30_000);
    const src = await img.getAttribute("src");
    if (!src.startsWith("http")) throw new Error(`image src is ${src.slice(0, 40)}`);
    const thumbUrl = src + (src.includes("?") ? "&" : "?") + "thumb=1";
    const type = await poll(
      () => guest.page.evaluate(async (u) => (await fetch(u)).headers.get("content-type"), thumbUrl),
      (t) => t && !t.startsWith("image/png"),
      60_000, 2000,
    );
    return `thumb is ${type}`;
  });
  await snap(guest, "chat-uploaded");
} catch (e) {
  console.log(`RESULT\tFAIL\t${mode} aborted\t-\t${String(e?.message ?? e).split("\n")[0]}`);
  failures++;
} finally {
  await browser.close();
}
process.exit(failures ? 1 : 0);
