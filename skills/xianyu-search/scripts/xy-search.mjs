// 闲鱼网页版（goofish.com）免登录搜索与商品采集。
//
// 关键前提（实测，别踩）：
//   1. 必须用页面上的搜索框输入关键词再回车。直接打开 /search?q=xxx 只会拿到
//      "小闲鱼没有找到你想要的宝贝" + 猜你喜欢兜底流，任何关键词都没有结果。
//   2. 每次打开新页面都会弹登录框并拦截点击，靠 xy-auto-close.mjs 自动关掉。
//
// 用法：
//   const { searchXianyu } = await import("<skill>/scripts/xy-search.mjs");
//   await searchXianyu({ query: "多邻国会员", limit: 30, details: 3 });
//   await searchXianyu({ queries: ["多邻国", "duolingo"], limit: 20 });
//
// 返回（同时打印 JSON）：{ spaceId, page, results: [{ query, url, count, items, details }] }

import { attach, attachTabs, waitClear } from "./xy-auto-close.mjs";

const HOME = "https://www.goofish.com/";
const CARD = 'a[href*="/item?"]';
const EMPTY_MARK = "没有找到你想要的宝贝";
const SPACE_NAME = "闲鱼商品调研";

async function pickPage(task) {
  const pages = await task.pages();
  if (pages.length) {
    const searchish = pages.find((pg) => /goofish\.com/.test(pg.url?.() || ""));
    return searchish || pages[0];
  }
  return task.newPage();
}

// 把搜索框打上标记，避免依赖会变的 class 哈希
async function tagSearchBox(page) {
  const ok = await page.evaluate(() => {
    document.querySelectorAll("[data-xy-search]").forEach((n) => n.removeAttribute("data-xy-search"));
    const inputs = [...document.querySelectorAll("input")].filter((i) => {
      const r = i.getBoundingClientRect();
      if (r.width < 200 || r.height < 20 || r.top < 0 || r.top > 180) return false;
      if (i.type === "hidden" || i.disabled) return false;
      const s = getComputedStyle(i);
      return s.visibility !== "hidden" && s.display !== "none";
    });
    if (!inputs.length) return false;
    inputs[0].setAttribute("data-xy-search", "1");
    return true;
  });
  return ok;
}

async function waitForResults(page, timeout = 25000) {
  const started = Date.now();
  while (Date.now() - started < timeout) {
    const state = await page.evaluate(({ card, mark }) => {
      if (document.querySelectorAll(card).length > 0) return "items";
      if (document.body.innerText.includes(mark)) return "empty";
      return "loading";
    }, { card: CARD, mark: EMPTY_MARK });
    if (state !== "loading") return state;
    await page.waitForTimeout(1200);
  }
  return "timeout";
}

async function readItems(page, limit) {
  return page.evaluate(({ card, limit }) => {
    const abs = (h) => (!h ? "" : h.startsWith("http") ? h : location.origin + h);
    return [...document.querySelectorAll(card)].slice(0, limit).map((a) => {
      const href = abs(a.getAttribute("href") || "");
      const text = (a.innerText || "").replace(/\s+/g, " ").trim();
      const titleEl = a.querySelector('[class*="main-title"], [class*="row1-wrap-title"], [class*="item-title"]');
      const img = a.querySelector("img");
      const price = (text.match(/¥\s*([\d.]+)/) || [])[1] || "";
      const want = (text.match(/([\d.]+)人想要/) || [])[1] || "";
      return {
        id: (href.match(/[?&]id=(\d+)/) || [])[1] || "",
        title: ((titleEl && titleEl.innerText) || text).replace(/\s+/g, " ").trim(),
        price: price ? Number(price) : null,
        want: want ? Number(want) : null,
        url: href,
        image: ((img && (img.currentSrc || img.src)) || "").replace(/(\.jpg).*$/, "$1"),
        text,
      };
    });
  }, { card: CARD, limit });
}

async function readDetail(page) {
  // 描述长的时候要点"展开"
  await page.evaluate(() => {
    const btn = [...document.querySelectorAll("div,span,button,a")]
      .find((n) => /^展开$/.test((n.textContent || "").trim()));
    if (btn) btn.click();
  }).catch(() => {});
  await page.waitForTimeout(700);
  return page.evaluate(() => {
    const pick = (sel) => {
      const n = document.querySelector(sel);
      return n ? (n.innerText || "").replace(/\s+/g, " ").trim() : "";
    };
    const scope = document.querySelector('[class*="item-main-info"]') || document.body;
    const scopeText = (scope.innerText || "").replace(/\s+/g, " ");
    // 描述元素自己也是 desc--xxx，但同前缀还有平台提示文字，取最长的那条
    const descs = [...document.querySelectorAll('[class^="desc"], [class*=" item-desc"], [class*="item-desc"]')]
      .map((n) => (n.innerText || "").replace(/\s+/g, " ").trim())
      .filter(Boolean)
      .sort((a, b) => b.length - a.length);
    const raw = [...document.querySelectorAll("img")]
      .map((i) => {
        const r = i.getBoundingClientRect();
        return { src: i.currentSrc || i.src || "", area: r.width * r.height };
      })
      .filter((x) => /bao\/uploaded/.test(x.src))
      .sort((a, b) => b.area - a.area);
    // 只保留和主图同量级的图，避免把页面下方"为你推荐"的缩略图混进来
    const biggest = raw.length ? raw[0].area : 0;
    const images = raw
      .filter((x) => x.area >= biggest * 0.5)
      .map((x) => x.src.replace(/(\.jpg).*$/, "$1"));
    return {
      url: location.href,
      title: document.title.replace(/_闲鱼$/, "").trim(),
      price: (scopeText.match(/¥\s*([\d.]+)/) || [])[1] || "",
      wants: (scopeText.match(/([\d.]+)人想要/) || [])[1] || "",
      views: (scopeText.match(/([\d.]+)浏览/) || [])[1] || "",
      seller: pick('[class*="item-user-info-nick"]'),
      sellerMeta: pick('[class*="item-user-info-intro"]'),
      desc: descs[0] || "",
      images: [...new Set(images)].slice(0, 12),
    };
  });
}

async function openDetails(page, items, count, detailDir) {
  const out = [];
  if (detailDir) {
    const fs = await import("node:fs/promises");
    await fs.mkdir(detailDir, { recursive: true });
  }
  for (let i = 0; i < Math.min(count, items.length); i += 1) {
    const clear = await waitClear(page, { selector: CARD });
    if (!clear) {
      out.push({ index: i, id: items[i].id, error: "列表被遮挡，跳过" });
      continue;
    }
    const popupPromise = page.waitForEvent("popup", { timeout: 10000 }).catch(() => null);
    try {
      await page.click(`${CARD} >> nth=${i}`, { label: `open item ${i}` });
    } catch (e) {
      out.push({ index: i, id: items[i].id, error: "点击失败: " + e.message.slice(0, 120) });
      continue;
    }
    const popup = await popupPromise;
    if (!popup) {
      out.push({ index: i, id: items[i].id, error: "未打开详情页" });
      continue;
    }
    await popup.waitForLoadState().catch(() => {});
    await attach(popup);
    await popup.waitForTimeout(3200);
    const detail = await readDetail(popup).catch((e) => ({ error: e.message }));
    if (detailDir && detail.images && detail.images[0]) {
      const file = `${detailDir}/${detail.id || items[i].id || i}.jpg`;
      const saved = await popup.fetch(detail.images[0], { saveAs: file, timeout: 25000 }).catch(() => null);
      if (saved) detail.coverImage = file;
    }
    await popup.close().catch(() => {});
    out.push({ index: i, id: items[i].id, ...detail });
    await page.waitForTimeout(500);
  }
  return out;
}

async function runOne(page, keyword, limit, details, detailDir) {
  // 首次或页面不在闲鱼时，先回首页拿到搜索框
  if (!/goofish\.com/.test(await page.url())) {
    await page.goto(HOME);
    await page.waitForLoadState().catch(() => {});
    await page.waitForTimeout(2000);
  }
  const tagged = await tagSearchBox(page);
  if (!tagged) {
    return { query: keyword, ok: false, reason: "没找到搜索框，检查页面是否已加载", url: await page.url(), items: [] };
  }
  await page.fill('[data-xy-search="1"]', keyword);
  await page.press('[data-xy-search="1"]', "Enter");
  const state = await waitForResults(page);
  const url = await page.url();
  if (state !== "items") {
    return {
      query: keyword,
      ok: false,
      reason: state === "empty"
        ? "搜索结果为空。直接拼 URL 打开搜索页会出现这个结果，务必用搜索框输入并回车"
        : "等待结果超时",
      url,
      items: [],
    };
  }
  const items = await readItems(page, limit);
  const result = { query: keyword, ok: true, url, count: items.length, items };
  if (details > 0) result.details = await openDetails(page, items, details, detailDir);
  return result;
}

export async function searchXianyu(options = {}) {
  const {
    query,
    queries,
    limit = 30,
    details = 0,
    spaceId = null,
    spaceName = SPACE_NAME,
    detailDir = null,
  } = options;
  const keywords = Array.isArray(queries) && queries.length ? queries : query ? [query] : [];
  if (!keywords.length) throw new Error("需要传入 query 或 queries");

  const task = spaceId ? await taskSpace(spaceId) : await taskSpace(spaceName);
  await attachTabs(task);
  const page = await pickPage(task);
  await attach(page);

  const results = [];
  for (const keyword of keywords) {
    results.push(await runOne(page, keyword, limit, details, detailDir));
  }
  const summary = { spaceId: task.spaceId, page: page.label, results };
  console.log(JSON.stringify(summary, null, 2));
  return { task, page, ...summary };
}

// 收尾：调研结束时关掉浏览器空间
export async function finishXianyu(task) {
  if (task && typeof task.finish === "function") await task.finish({ keep: [] });
}
