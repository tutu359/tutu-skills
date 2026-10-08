// 闲鱼网页版（goofish.com）登录弹窗自动关闭工具。
//
// 为什么需要它：闲鱼网页版每次打开新页面（含整页刷新、SPA 路由切换、点开商品详情的新标签）
// 都会弹出登录框，弹窗的遮罩会拦截点击，导致元素"可见但点不到"。
//
// 用法（ego-browser nodejs 脚本里）：
//   const { attach, attachTabs, waitClear } = await import("<skill>/scripts/xy-auto-close.mjs");
//   await attach(page);        // 单页：立刻关闭 + 之后该页所有新文档都自动关闭
//   await attachTabs(task);    // TaskSpace 内所有已打开页面
//   await attach(newPage);     // 新开的标签页必须单独 attach 一次
//   await waitClear(page);     // 等到页面真的可以点击再动手

export const AUTO_CLOSE_SOURCE = `(() => {
  const VERSION = "1";
  if (window.__xyCloseVersion === VERSION) return "already";
  window.__xyCloseVersion = VERSION;
  window.__xyCloseInstalled = true;
  if (window.__xyTimer) { clearInterval(window.__xyTimer); window.__xyTimer = null; }
  if (window.__xyObserver) { try { window.__xyObserver.disconnect(); } catch (e) {} window.__xyObserver = null; }
  window.__xyHistoryHooked = false;

  const CLOSE_SELECTORS = [
    '.baxia-dialog-close',
    '.ant-modal-close',
    '[class*="dialog-close"]',
    '[class*="modal-close"]',
    '[class*="closeIcon"]',
    '[class*="close-icon"]',
    '[class*="closeBtn"]',
    '[class*="close-btn"]',
    '[class*="dialog"] [class*="close"]',
    '[class*="modal"] [class*="close"]',
    '[class*="loginCon"] [class*="close"]',
  ];
  const MASK_SELECTORS = ['.ant-modal-mask', '.baxia-dialog-mask', '.login-iframe-wrap'];
  const MODAL_SELECTORS = ['.login-modal-wrap', '.ant-modal-wrap', '.baxia-dialog'];

  const visible = (n) => {
    if (!n || !n.getBoundingClientRect) return false;
    const r = n.getBoundingClientRect();
    if (r.width <= 0 || r.height <= 0) return false;
    const s = getComputedStyle(n);
    return s.display !== 'none' && s.visibility !== 'hidden' && s.opacity !== '0';
  };

  // 关掉弹窗后遮罩层有时还留着，继续拦截点击
  const dropMasks = () => {
    MASK_SELECTORS.forEach((sel) => {
      document.querySelectorAll(sel).forEach((m) => {
        const r = m.getBoundingClientRect();
        if (r.width > 200 && r.height > 200 && getComputedStyle(m).pointerEvents !== 'none') {
          m.style.pointerEvents = 'none';
        }
      });
    });
  };

  const isLoginModal = (el) => {
    const t = el.innerText || '';
    return /请输入手机号|扫码安全登录|短信登录|密码登录|获取验证码/.test(t) || !!el.querySelector('iframe');
  };
  const visibleLoginModals = () => {
    return [...document.querySelectorAll(MODAL_SELECTORS.join(','))].filter((w) => {
      const r = w.getBoundingClientRect();
      if (r.width < 300 || r.height < 250) return false;
      const s = getComputedStyle(w);
      if (s.display === 'none' || s.visibility === 'hidden') return false;
      return isLoginModal(w);
    });
  };

  // 先点关闭按钮；点不掉就整体隐藏，保证页面可点
  let stuck = 0;
  const hideIfStuck = () => {
    const modals = visibleLoginModals();
    if (!modals.length) { stuck = 0; return 0; }
    stuck += 1;
    if (stuck < 2) return 0;
    modals.forEach((w) => {
      w.style.display = 'none';
      w.style.pointerEvents = 'none';
      document.querySelectorAll(MASK_SELECTORS.join(',')).forEach((m) => {
        m.style.display = 'none';
        m.style.pointerEvents = 'none';
      });
    });
    return modals.length;
  };

  window.__xyKill = () => {
    let n = 0;
    for (const sel of CLOSE_SELECTORS) {
      document.querySelectorAll(sel).forEach((b) => {
        if (visible(b)) { try { b.click(); n += 1; } catch (e) {} }
      });
    }
    document.querySelectorAll('[class*="login-tip" i] [class*="close" i], [class*="loginTip" i] [class*="close" i]').forEach((b) => {
      if (visible(b)) { try { b.click(); n += 1; } catch (e) {} }
    });
    dropMasks();
    n += hideIfStuck();
    return n;
  };

  if (!window.__xyTimer) window.__xyTimer = setInterval(window.__xyKill, 400);
  if (!window.__xyObserver) {
    window.__xyObserver = new MutationObserver(() => window.__xyKill());
    window.__xyObserver.observe(document.documentElement, { childList: true, subtree: true });
  }
  if (!window.__xyHistoryHooked) {
    window.__xyHistoryHooked = true;
    const wrap = (fn) => function () {
      const out = fn.apply(this, arguments);
      [60, 400, 1200, 2500].forEach((d) => setTimeout(window.__xyKill, d));
      return out;
    };
    history.pushState = wrap(history.pushState);
    history.replaceState = wrap(history.replaceState);
    window.addEventListener('popstate', () => [120, 700, 1800].forEach((d) => setTimeout(window.__xyKill, d)));
    window.addEventListener('load', () => [80, 500, 1500, 3000].forEach((d) => setTimeout(window.__xyKill, d)));
  }
  window.__xyKill();
  return "installed";
})()`;

// 让该页面之后加载的每个新文档都自带关闭脚本（整页刷新也不会丢）
export async function attach(page) {
  try {
    await page.cdp("Page.addScriptToEvaluateOnNewDocument", { source: AUTO_CLOSE_SOURCE });
  } catch (e) {
    // 注入失败时下面还有一次直接注入兜底
  }
  try {
    await page.evaluate(AUTO_CLOSE_SOURCE);
  } catch (e) {
    // 页面还没准备好时忽略
  }
  return page;
}

export async function attachTabs(task) {
  const tabs = await task.tabs();
  for (const t of tabs) {
    if (t && t.page) await attach(t.page);
  }
  return tabs;
}

export async function kick(page) {
  try {
    await page.evaluate(() => (window.__xyKill ? window.__xyKill() : 0));
  } catch (e) {}
  return page;
}

// 等到"页面真的能被点"：隐藏登录弹窗与遮罩，并确认目标元素上方没有别的东西挡着
export async function waitClear(page, options = {}) {
  const { selector = null, timeout = 8000 } = options;
  const started = Date.now();
  while (Date.now() - started < timeout) {
    let clear = false;
    try {
      clear = await page.evaluate((sel) => {
        const isVisible = (w) => {
          const r = w.getBoundingClientRect();
          const s = getComputedStyle(w);
          return r.width > 300 && r.height > 250 && s.display !== 'none' && s.visibility !== 'hidden';
        };
        const modals = [...document.querySelectorAll('.login-modal-wrap, .ant-modal-wrap, .baxia-dialog')].filter(isVisible);
        if (modals.length) {
          modals.forEach((w) => { w.style.display = 'none'; w.style.pointerEvents = 'none'; });
          document.querySelectorAll('.ant-modal-mask, .baxia-dialog-mask, .login-iframe-wrap').forEach((m) => {
            m.style.display = 'none';
            m.style.pointerEvents = 'none';
          });
        }
        if (!sel) return modals.length === 0;
        const target = document.querySelector(sel);
        if (!target) return false;
        const r = target.getBoundingClientRect();
        if (r.width === 0 || r.height === 0) return false;
        const top = document.elementFromPoint(Math.round(r.x + r.width / 2), Math.round(r.y + r.height / 2));
        if (!top) return true;
        return top === target || target.contains(top) || top.contains(target);
      }, selector);
    } catch (e) {
      return false;
    }
    if (clear) return true;
    await page.waitForTimeout(300);
  }
  return false;
}
