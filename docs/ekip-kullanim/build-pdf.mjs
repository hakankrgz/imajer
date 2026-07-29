#!/usr/bin/env node

import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, delimiter, dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const projectDir = resolve(scriptDir, "../..");
const input = resolve(process.argv[2] || join(projectDir, "EKIP_HIZLI_KULLANIM.md"));
const output = resolve(process.argv[3] || join(projectDir, "EKIP_HIZLI_KULLANIM.pdf"));
function browserExecutable() {
  const candidates = [
    process.env.IMAJER_CHROME,
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
    join(process.env.PROGRAMFILES || "", "Microsoft", "Edge", "Application", "msedge.exe"),
    join(process.env["PROGRAMFILES(X86)"] || "", "Microsoft", "Edge", "Application", "msedge.exe"),
    join(process.env.LOCALAPPDATA || "", "Microsoft", "Edge", "Application", "msedge.exe"),
    "/usr/bin/google-chrome",
    "/usr/bin/chromium",
    "/usr/bin/chromium-browser",
    "/usr/bin/microsoft-edge",
  ].filter(Boolean);
  for (const candidate of candidates) {
    if (existsSync(candidate)) return candidate;
  }
  for (const name of process.platform === "win32"
    ? ["msedge.exe", "chrome.exe"]
    : ["google-chrome", "chromium", "microsoft-edge"]) {
    for (const directory of (process.env.PATH || "").split(delimiter)) {
      const candidate = join(directory, name);
      if (existsSync(candidate)) return candidate;
    }
  }
  throw new Error("Chrome/Chromium/Edge bulunamadı; IMAJER_CHROME ile executable yolunu belirtin.");
}

const chrome = browserExecutable();

const escapeHTML = (value) => value
  .replaceAll("&", "&amp;")
  .replaceAll("<", "&lt;")
  .replaceAll(">", "&gt;")
  .replaceAll('"', "&quot;");

function inline(value) {
  let html = escapeHTML(value);
  html = html.replace(/`([^`]+)`/g, "<code>$1</code>");
  html = html.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
  html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2">$1</a>');
  return html;
}

function markdownToHTML(markdown) {
  const lines = markdown.replaceAll("\r\n", "\n").split("\n");
  const out = [];
  let paragraph = [];
  let listType = "";
  let listItems = [];
  let quote = [];
  let code = [];
  let inCode = false;

  const flushParagraph = () => {
    if (paragraph.length) {
      out.push(`<p>${inline(paragraph.join(" "))}</p>`);
      paragraph = [];
    }
  };
  const flushList = () => {
    if (listItems.length) {
      out.push(`<${listType}>${listItems.map((item) => `<li>${inline(item)}</li>`).join("")}</${listType}>`);
      listItems = [];
      listType = "";
    }
  };
  const flushQuote = () => {
    if (quote.length) {
      out.push(`<blockquote><p>${inline(quote.join(" "))}</p></blockquote>`);
      quote = [];
    }
  };
  const flushText = () => {
    flushParagraph();
    flushList();
    flushQuote();
  };

  for (const sourceLine of lines) {
    const line = sourceLine.replace(/\s+$/, "");
    if (line.startsWith("```")) {
      flushText();
      if (inCode) {
        out.push(`<pre><code>${escapeHTML(code.join("\n"))}</code></pre>`);
        code = [];
      }
      inCode = !inCode;
      continue;
    }
    if (inCode) {
      code.push(sourceLine);
      continue;
    }
    if (!line.trim()) {
      flushText();
      continue;
    }
    const image = line.match(/^!\[([^\]]*)\]\(([^)]+)\)$/);
    if (image) {
      flushText();
      out.push(`<figure><img src="${escapeHTML(image[2])}" alt="${escapeHTML(image[1])}"><figcaption>${inline(image[1])}</figcaption></figure>`);
      continue;
    }
    const heading = line.match(/^(#{1,4})\s+(.+)$/);
    if (heading) {
      flushText();
      const level = heading[1].length;
      const pageBreak = ["0.6.5 doğrulama özeti", "2. Yeni işlem oluşturun"].includes(heading[2])
        ? ' class="page-break"'
        : "";
      out.push(`<h${level}${pageBreak}>${inline(heading[2])}</h${level}>`);
      continue;
    }
    if (line.startsWith(">")) {
      flushParagraph();
      flushList();
      quote.push(line.replace(/^>\s?/, ""));
      continue;
    }
    const unordered = line.match(/^\s*-\s+(.+)$/);
    const ordered = line.match(/^\s*\d+\.\s+(.+)$/);
    if (unordered || ordered) {
      flushParagraph();
      flushQuote();
      const nextType = ordered ? "ol" : "ul";
      if (listType && listType !== nextType) flushList();
      listType = nextType;
      listItems.push((unordered || ordered)[1]);
      continue;
    }
    if (listItems.length && /^\s{2,}\S/.test(sourceLine)) {
      listItems[listItems.length - 1] += ` ${line.trim()}`;
      continue;
    }
    flushList();
    flushQuote();
    paragraph.push(line.trim());
  }
  flushText();
  if (inCode) out.push(`<pre><code>${escapeHTML(code.join("\n"))}</code></pre>`);
  return out.join("\n");
}

const markdown = readFileSync(input, "utf8");
const content = markdownToHTML(markdown);
const baseURL = pathToFileURL(`${projectDir}/`).href;
const generated = new Intl.DateTimeFormat("tr-TR", {
  dateStyle: "long",
  timeZone: "Europe/Istanbul",
}).format(new Date());
const title = "IMAJER 0.6.8 — Proje Özeti ve Ekip Hızlı Kullanım";
const html = `<!doctype html>
<html lang="tr">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<base href="${baseURL}">
<title>${title}</title>
<style>
  @page { size: A4; margin: 16mm 15mm 17mm; }
  * { box-sizing: border-box; }
  html { color: #172033; background: #fff; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Arial, sans-serif; font-size: 10.5pt; }
  body { margin: 0; background: #fff; line-height: 1.48; }
  h1 { margin: 0 0 10mm; padding: 16mm 12mm; border-radius: 5mm; color: white; background: linear-gradient(135deg, #102a43, #126e82); font-size: 26pt; line-height: 1.15; }
  h1::after { content: "Ekran görüntülü kısa kullanım, adli bütünlük ve hata yorumlama rehberi"; display: block; margin-top: 5mm; color: #d9f5f2; font-size: 11pt; font-weight: 500; }
  h2 { margin: 8mm 0 3mm; padding-bottom: 1.5mm; border-bottom: 1.4pt solid #2aa198; color: #0f4c5c; font-size: 17pt; break-after: avoid; }
  h3 { margin: 5mm 0 2mm; color: #16697a; font-size: 13pt; break-after: avoid; }
  h4 { margin: 4mm 0 1.5mm; font-size: 11pt; break-after: avoid; }
  .page-break { break-before: page; }
  p { margin: 0 0 3.2mm; orphans: 3; widows: 3; }
  ul, ol { margin: 1.5mm 0 4mm 6mm; padding-left: 5mm; }
  li { margin: 0 0 1.6mm; break-inside: avoid; }
  blockquote { margin: 4mm 0 6mm; padding: 3.5mm 5mm; border-left: 4pt solid #e9c46a; border-radius: 2mm; background: #fff8e6; }
  blockquote p { margin: 0; font-weight: 600; }
  code { padding: .3mm 1mm; border-radius: 1mm; background: #edf2f7; color: #7b2d26; font-family: "SFMono-Regular", Consolas, monospace; font-size: 9pt; }
  pre { margin: 3mm 0 5mm; padding: 4mm; border: 1px solid #cbd5e0; border-radius: 2mm; background: #f7fafc; white-space: pre-wrap; break-inside: avoid; }
  pre code { padding: 0; background: transparent; color: #1a202c; }
  figure { margin: 5mm 0 7mm; padding: 3mm; border: 1px solid #ccd6e0; border-radius: 3mm; background: #f8fafc; break-inside: avoid; }
  figure img { display: block; width: 100%; height: auto; border-radius: 2mm; }
  figcaption { margin-top: 2mm; color: #52616b; font-size: 8.5pt; text-align: center; }
  a { color: #126e82; text-decoration: none; }
  strong { color: #102a43; }
  .meta { margin: -6mm 0 8mm; color: #52616b; font-size: 8.5pt; text-align: right; }
  .footer { margin-top: 10mm; padding-top: 3mm; border-top: 1px solid #cbd5e0; color: #52616b; font-size: 8pt; text-align: center; }
</style>
</head>
<body>
${content}
<div class="footer">IMAJER 0.6.8 · Güncelleme: ${escapeHTML(generated)} · Kaynak: ${escapeHTML(basename(input))}</div>
</body>
</html>`;

const tempDir = mkdtempSync(join(tmpdir(), "imajer-pdf-"));
const htmlPath = join(tempDir, "ekip-hizli-kullanim.html");
try {
  writeFileSync(htmlPath, html, "utf8");
  execFileSync(chrome, [
    "--headless=new",
    "--disable-gpu",
    "--allow-file-access-from-files",
    "--no-pdf-header-footer",
    `--print-to-pdf=${output}`,
    pathToFileURL(htmlPath).href,
  ], { stdio: "inherit" });
} finally {
  rmSync(tempDir, { recursive: true, force: true });
}
