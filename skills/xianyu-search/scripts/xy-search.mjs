// 闲鱼网页版（goofish.com）免登录搜索与商品采集。
//
// 关键前提（实测，别踩）：
//   1. 必须用页面上的搜索框输入关键词再回车。直接打开 /search?q=xxx 只会拿到
//      "小闲鱼没有找到你想要的宝贝" + 猜你喜欢兜底流，任何关键词都没有结果。
//   2. 每次打开新页面都会弹登录框并拦截点击，靠 xy-auto-close.mjs 自动关掉。
//   3. 回车后页面会真的换一次文档，切换瞬间 document.body 是 null。所有读取
//      页面的地方都必须走 safeEvaluate，不能直接用 document.body.innerText。
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

// 页面导航的瞬间执行 evaluate，拿到的是还没解析出 body 的新文档，会抛
// TypeError: Cannot read properties of null (reading 'innerText')。
// 这是竞态不是错误用法，所以统一重试，不把瞬时状态抛给调用方。
async function safeEvaluate(page, fn, arg, options = {}) {
  const { retries = 6, delay = 350, fallback } = options;
  let lastError = null;
  for (let i = 0; i < retries; i += 1) {
    try {
      // ego-browser 里 evaluate 的第二个参数必须能 JSON 序列化，
      // 传 undefined 会直接抛 "argument must be JSON-serializable"。
      return arg === undefined ? await page.evaluate(fn) : await page.evaluate(fn, arg);
    } catch (e) {
      lastError = e;
      await page.waitForTimeout(delay).catch(() => {});
    }
  }
  if (fallback !== undefined) return fallback;
  throw lastError;
}

async function currentUrl(page) {
  try {
    return await page.url();
  } catch (e) {
    return "";
  }
}

async function pickPage(task) {
  const pages = await task.pages();
  if (!pages.length) return task.newPage();
  for (const candidate of pages) {
    if (/goofish\.com/.test(await currentUrl(candidate))) return candidate;
  }
  return pages[0];
}

// 给搜索框打标记再定位，避免依赖会随构建变化的 class 哈希
async function tagSearchBox(page, timeout = 12000) {
  const started = Date.now();
  while (Date.now() - started < timeout) {
    const ok = await safeEvaluate(page, () => {
      if (!document || !document.body) return false;
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
    }, undefined, { retries: 2, fallback: false });
    if (ok) return true;
    await page.waitForTimeout(400);
  }
  return false;
}

async function waitForResults(page, timeout = 25000) {
  const started = Date.now();
  let emptyStreak = 0;
  while (Date.now() - started < timeout) {
    const probe = await safeEvaluate(page, ({ card, mark }) => {
      if (!document || !document.body) return { hasMark: false, cards: 0 };
      return {
        hasMark: (document.body.innerText || "").includes(mark),
        cards: document.querySelectorAll(card).length,
      };
    }, { card: CARD, mark: EMPTY_MARK }, { retries: 3, fallback: { hasMark: false, cards: 0 } });
    // 必须先用"没有找到"判定：查不到时页面会显示这句提示，同时给出 20 条
    // "猜你喜欢"推荐流，而推荐流用的也是 /item? 链接。先看卡片数会把推荐流
    // 当成搜索结果返回，静默产出垃圾数据。
    if (probe.hasMark) {
      emptyStreak += 1;
      if (emptyStreak >= 2) return "empty";
    } else {
      emptyStreak = 0;
      if (probe.cards > 0) return "items";
    }
    await page.waitForTimeout(1000);
  }
  return "timeout";
}

async function readItems(page, limit) {
  // 卡片图是懒加载，读太早会拿到 2x2 占位图，先给图一点时间
  await page.waitForTimeout(1200);
  return safeEvaluate(page, ({ card, limit }) => {
    // 闲鱼把 "¥ 2 .50" 渲染成分隔的小块，直接匹配数字会只拿到整数部分
    const norm = (s) => String(s || "").replace(/(\d)\s+(?=\.)/g, "$1");
    const abs = (h) => (!h ? "" : h.startsWith("http") ? h : location.origin + h);
    return [...document.querySelectorAll(card)].slice(0, limit).map((a) => {
      const href = abs(a.getAttribute("href") || "");
      const text = (a.innerText || "").replace(/\s+/g, " ").trim();
      const flat = norm(text);
      const titleEl = a.querySelector('[class*="main-title"], [class*="row1-wrap-title"], [class*="item-title"]');
      const img = a.querySelector("img");
      const raw = img ? img.currentSrc || img.src || "" : "";
      const image = raw.replace(/(\.jpg).*$/, "$1");
      // 只在价格元素里取价，避免标题里出现的金额（例如"官网588""¥2.5起"）被当成售价
      const priceEl = a.querySelector('[class*="row3-wrap-price"], [class*="price-wrap"]');
      const priceSrc = norm(priceEl ? priceEl.innerText || "" : "");
      const price = (priceSrc.match(/¥\s*([\d.]+)/) || flat.match(/¥\s*([\d.]+)/) || [])[1] || "";
      const want = (flat.match(/([\d.]+)人想要/) || [])[1] || "";
      return {
        id: (href.match(/[?&]id=(\d+)/) || [])[1] || "",
        title: ((titleEl && titleEl.innerText) || text).replace(/\s+/g, " ").trim(),
        // 多规格商品在列表里只显示最低档价格，区间要进详情页才有
        price: price ? Number(price) : null,
        want: want ? Number(want) : null,
        url: href,
        // /bao/uploaded/ 才是真图，其它都是占位或图标
        image: /bao\/uploaded/.test(image) ? image : "",
        text,
      };
    });
  }, { card: CARD, limit }, { retries: 4, fallback: [] });
}

async function readDetail(page) {
  // 描述长的时候要点"展开"
  await safeEvaluate(page, () => {
    if (!document || !document.body) return false;
    const btn = [...document.querySelectorAll("div,span,button,a")]
      .find((n) => /^展开$/.test((n.textContent || "").trim()));
    if (btn) btn.click();
    return true;
  }, undefined, { retries: 3, fallback: false });
  await page.waitForTimeout(700);
  return safeEvaluate(page, () => {
    const norm = (s) => String(s || "").replace(/(\d)\s+(?=\.)/g, "$1");
    const pick = (sel) => {
      const n = document.querySelector(sel);
      return n ? (n.innerText || "").replace(/\s+/g, " ").trim() : "";
    };
    const scope = document.querySelector('[class*="item-main-info"]') || document.body;
    const scopeText = norm((scope.innerText || "").replace(/\s+/g, " "));
    // 多规格商品的售价是一个区间，且渲染在独立的 price 元素里（"14.9 - 99.9"，¥ 在另一个节点）
    const priceEl = document.querySelector('[class*="price--"]');
    const priceText = norm(priceEl ? (priceEl.innerText || "").trim() : "");
    const range = priceText.match(/(\d+(?:\.\d+)?)\s*(?:-|~|～|至|到)\s*(?:¥\s*)?(\d+(?:\.\d+)?)/);
    const single = priceText.match(/(\d+(?:\.\d+)?)/);
    let priceMin = null;
    let priceMax = null;
    if (range) {
      priceMin = Math.min(Number(range[1]), Number(range[2]));
      priceMax = Math.max(Number(range[1]), Number(range[2]));
    } else if (single) {
      priceMin = Number(single[1]);
      priceMax = priceMin;
    } else {
      const fallback = scopeText.match(/¥\s*([\d.]+)/);
      if (fallback) {
        priceMin = Number(fallback[1]);
        priceMax = priceMin;
      }
    }
    // 描述元素自己也是 desc--xxx，同前缀还有平台提示文字，取最长的那条
    const descs = [...document.querySelectorAll('[class^="desc"], [class*="item-desc"]')]
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
    // 只保留和主图同量级的图，避免把"为你推荐"的缩略图混进来
    const biggest = raw.length ? raw[0].area : 0;
    const images = raw
      .filter((x) => x.area >= biggest * 0.5)
      .map((x) => x.src.replace(/(\.jpg).*$/, "$1"));
    return {
      url: location.href,
      title: document.title.replace(/_闲鱼$/, "").trim(),
      price: priceMin === null ? "" : String(priceMin),
      priceMin,
      priceMax,
      priceText,
      // 是否多规格（价格区间）：PC 网页不提供可点击的规格选择器，规格只在描述里
      hasPriceRange: !!(range && priceMax > priceMin),
      wants: (scopeText.match(/([\d.]+)人想要/) || [])[1] || "",
      views: (scopeText.match(/([\d.]+)浏览/) || [])[1] || "",
      seller: pick('[class*="item-user-info-nick"]'),
      sellerMeta: pick('[class*="item-user-info-intro"]'),
      desc: descs[0] || "",
      images: [...new Set(images)].slice(0, 12),
    };
  }, undefined, { retries: 5, fallback: { error: "详情页读取失败" } });
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

async function attemptSearch(page, keyword) {
  if (!/goofish\.com/.test(await currentUrl(page))) {
    await page.goto(HOME);
    await page.waitForLoadState().catch(() => {});
    await page.waitForTimeout(2000);
  }
  if (!(await tagSearchBox(page))) return { state: "no-box" };
  await page.fill('[data-xy-search="1"]', keyword);
  await page.press('[data-xy-search="1"]', "Enter");
  // 回车后页面会换文档，等它站稳再开始读
  await page.waitForTimeout(900);
  const state = await waitForResults(page);
  return { state, url: await currentUrl(page) };
}

async function runOne(page, keyword, limit, details, detailDir) {
  let attempt = await attemptSearch(page, keyword);
  // 搜索框还没渲染出来时补一次，避免把竞态当成"页面结构变了"
  if (attempt.state === "no-box" || attempt.state === "timeout") {
    await page.waitForTimeout(1500);
    attempt = await attemptSearch(page, keyword);
  }
  if (attempt.state !== "items") {
    const reason = attempt.state === "empty"
      ? "搜索结果为空（页面提示『没有找到你想要的宝贝』，只会给推荐流）。要么这个关键词确实没有商品，要么是直接拼 URL 打开了搜索页；确认走的是搜索框输入回车。"
      : attempt.state === "no-box"
        ? "没找到搜索框，页面可能没加载完"
        : "等待结果超时";
    return { query: keyword, ok: false, reason, url: attempt.url || (await currentUrl(page)), items: [] };
  }
  const items = await readItems(page, limit);
  const result = { query: keyword, ok: true, url: attempt.url, count: items.length, items };
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
  // page 字段是给 JSON 看的标签，要操作页面用 pageObject
  return { task, pageObject: page, ...summary };
}

// 收尾：调研结束时关掉浏览器空间
export async function finishXianyu(task) {
  if (task && typeof task.finish === "function") await task.finish({ keep: [] });
}
