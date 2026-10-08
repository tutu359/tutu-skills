# 闲鱼网页版调研：常见问题

只在遇到对应症状时读本节，不用提前加载。

## 搜索结果为空

**表现**：`items` 数量为 0，页面显示「小闲鱼没有找到你想要的宝贝」加「猜你喜欢」，换任何关键词都一样。

**原因**：走的是 URL 跳转而不是搜索框。游客状态下直接打开 `/search?q=xxx` 不返回任何真实结果，只给推荐兜底流。

**处置**：确认脚本走的是「回首页 → 标记搜索框 → `fill` → `press Enter`」这条路径；如果自己写脚本，不要用 `page.goto` 到搜索 URL 来换关键词，要在结果页的搜索框里改词再回车。

**最容易踩的变体**：关键词**真的查不到**时，页面同样是「没有找到」+ 20 条猜你喜欢，而推荐流用的也是 `a[href*="/item?"]`。如果判断顺序是"先看有没有卡片、再看有没有那句提示"，就会把推荐流当成搜索结果返回，**不报错、静默产出 20 条完全不相关的垃圾数据**。所以判定必须先看提示文案（`xy-search.mjs` 里还要求连续两次都看到提示才算空，避免加载瞬间误判），再看卡片数量。

## evaluate 传了 undefined 参数

**表现**：脚本静默失效，比如"关弹窗/找搜索框"那一步一直返回 false，最后报"没找到搜索框"，但手动看页面明明正常。

**原因**：ego-browser 的 `page.evaluate(fn, arg)` 要求第二个参数可 JSON 序列化，传 `undefined` 会直接抛 `page.evaluate argument must be JSON-serializable`。如果外面包了 try/catch 或重试，这个错误就被吞掉了，表现为功能悄悄不工作。

**处置**：参数为 `undefined` 时改成单参数调用 `page.evaluate(fn)`。`xy-search.mjs` 的 `safeEvaluate` 已经处理了这一点，自己写脚本时注意。

## 回车后立刻读页面报 null

**表现**：`TypeError: Cannot read properties of null (reading 'innerText')`，位置在 `__egoPageEvaluate`。在已经处于搜索结果页时再次搜索、连跑第二次时更容易出现。

**原因**：在搜索框回车会真的换一次文档，切换当口 `document.body` 是 `null`，此时读 `document.body.innerText` 必崩。

**处置**：所有读取页面的 evaluate 都要先判 `if (!document || !document.body) return ...`，并且外层要有重试。`xy-search.mjs` 的 `safeEvaluate` + `waitForResults` 已经覆盖，回车后还会先等 900ms 再开始轮询。

## 价格少了小数、图片是占位图

**表现**：详情写 `¥ 2 .50`，脚本读出来是 `2`；`image` 字段是 `.../tps-2-2.png` 这种图标。

**原因**：闲鱼把价格的整数和小数拆成不同元素渲染，`innerText` 里会多出空格（`¥ 2 .50`），直接正则只匹配到整数；卡片图是懒加载，读太早拿到的是 2x2 占位图。

**处置**：读文本前先做 `replace(/(\d)\s+(?=\.)/g, "$1")` 归一化；图片只认 URL 里含 `/bao/uploaded/` 的，并在读取前留出约 1.2 秒让懒加载完成。

## 价格取到了标题里的数字

**表现**：卡片价格明显偏低，或者对不上商品。

**原因**：直接在整个卡片文本里找第一个 `¥`，而标题本身可能带金额（例如「和官网 588 的完全相同」「¥2.5 起」）。

**处置**：只在价格元素里取价——搜索卡片是 `[class*="row3-wrap-price"]`，详情页是 `[class*="price--"]`（注意推荐的 `price-wrap--` 不含 `price--` 这个子串，不会误命中）。

## 多规格商品的价格

**表现**：卡片显示 `¥14.90`，详情页却是 `14.9 - 99.9`；或者想知道「月付 / 季付 / 年付」各自多少钱。

**原因**：卡片只显示最低档价；区间只在详情页的价格元素里；规格选项 PC 网页不渲染成可点击控件。

**处置**：报价格用 `priceMin`–`priceMax` 区间，别把最低档当成售价；各档位的具体价格要去 `desc` 里读（卖家通常写「【可选规格】1. 月付 … 2. 季付 …」）。不要花时间找规格选择器，页面上没有；点击「立即购买」才可能弹出选择器，而那是下单流程，调研时不要点。

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
