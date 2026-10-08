# 闲鱼网页版调研：常见问题

只在遇到对应症状时读本节，不用提前加载。

## 搜索结果为空

**表现**：`items` 数量为 0，页面显示「小闲鱼没有找到你想要的宝贝」加「猜你喜欢」，换任何关键词都一样。

**原因**：走的是 URL 跳转而不是搜索框。游客状态下直接打开 `/search?q=xxx` 不返回任何真实结果，只给推荐兜底流。

**处置**：确认脚本走的是「回首页 → 标记搜索框 → `fill` → `press Enter`」这条路径；如果自己写脚本，不要用 `page.goto` 到搜索 URL 来换关键词，要在结果页的搜索框里改词再回车。

## 元素看得见点不到

**表现**：`page.click` 报错 `dialog intercepts pointer events` 或 `<div> intercepts pointer events`，元素明明已经渲染。

**原因**：登录弹窗的遮罩还在。闲鱼有两套登录弹窗：`.baxia-dialog`（阿里 Baxia 弹层）和 `.ant-modal-wrap.login-modal-wrap--*`（antd 弹窗，关闭按钮是 `.closeIconBg--*` / `img[class*="closeIcon"]`）。搜索或路由切换后弹窗会再次出现。

**处置**：点之前先 `await waitClear(page, { selector: 'a[href*="/item?"]' })`。它会隐藏弹窗和遮罩，并确认目标元素上方没有别的东西遮挡。返回 `false` 说明 8 秒内都没清干净，这时不要硬点，重新观察页面。

## 新标签页又开始弹窗

**表现**：搜索结果页正常，点进商品详情后弹窗又出来了。

**原因**：关闭脚本是按页面注入的，新标签页是全新的 page。

**处置**：拿到 popup 后立刻 `await attach(popup)` 再操作。`attachTabs(task)` 只能覆盖调用那一刻已经存在的标签页。

## 整页刷新后脚本失效

**原因**：只调用了一次 `page.evaluate(源码)`，刷新后文档重建，注入的内容没了。

**处置**：`attach()` 里已经同时调用了 `page.cdp("Page.addScriptToEvaluateOnNewDocument", ...)`，之后该页的每个新文档都会自动执行。自己写脚本时不要漏掉这一步。

## class 选择器突然失效

闲鱼前端用 CSS Modules，class 形如 `search-input--WY2l9QD3`、`item-main-info--ExVwW2NW`，哈希后缀会随发布变化。

**处置**：只用前缀匹配，例如 `[class*="search-input"]`、`[class*="item-main-info"]`；或者像 `xy-search.mjs` 那样先给元素打 `data-xy-search` 标记再定位。不要写死完整 class。

## 详情页取到的字段不对

- **标题**：`document.title` 才是商品标题（结尾多一个 `_闲鱼`，脚本已去掉）。页面上第一个 `[class*="title"]` 往往是「为你推荐」。
- **描述**：平台提示文字和卖家描述用了同一个 class 前缀（`desc--xxx`），要取文本最长的那一条，不能取第一个。
- **价格 / 想要人数 / 浏览量**：限定在 `[class*="item-main-info"]` 里匹配，否则会把推荐位商品的数据混进来。
- **主图**：取 `img[src*="bao/uploaded"]` 里面积最大的那张，并去掉 URL 尾部的 `_790x10000Q90.jpg_.webp` 之类缩略参数。
- **卖家**：`[class*="item-user-info-nick"]` 是昵称，`[class*="item-user-info-intro"]` 是地区、来闲鱼年限、卖出件数、好评率。

## 已知页面行为（2026-10-08 实测）

- 商品详情是 `target="_blank"`，一定用 `page.waitForEvent("popup")` 接住，不要指望当前页跳转。
- 描述过长时页面上有「展开」按钮，脚本会先点一次再抓全文。
- 顶部搜索框在首页和搜索结果页都存在，脚本复用同一页时不会重复跳首页。
- 未登录状态下页面底部会有一条「登录后可以更懂你」的提示条，关闭脚本也会顺手点掉。

## 遇到验证

出现人机验证、短信/扫码登录要求、风控提示或身份验证时：立即停止，不要输入账号密码验证码，不要换 IP、UA、代理或镜像重试，把页面情况告诉用户并等确认。
