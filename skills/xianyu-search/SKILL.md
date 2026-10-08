---
name: xianyu-search
description: 在闲鱼网页版（goofish.com）免登录搜索商品，采集搜索结果与商品详情（标题、价格、想要人数、详情文案、主图），并自动关闭每次打开新页面都会弹出的登录框。Use when the user asks to 调研闲鱼同类商品、搜索闲鱼、看看别人怎么卖、抓闲鱼商品标题/价格/文案/主图，or invokes $xianyu-search.
---

# 闲鱼搜索与商品调研

用 ego-browser 在闲鱼网页版做免登录调研。这个 skill 存在的唯一理由，是闲鱼网页版有两个让人反复踩坑的行为。

## 两个必须知道的前提

**一、搜索必须走搜索框。** 直接打开 `https://www.goofish.com/search?q=关键词` 只会得到「小闲鱼没有找到你想要的宝贝」加推荐兜底流，任何关键词都是空的（实测 `iPhone`、`苹果手机壳` 这类热词同样为空）。必须是**在搜索框里输入文字再回车**，才会出真实结果。脚本已经按这个方式实现，不要改成 URL 跳转。

**二、弹窗必须在场。** 每次打开新页面——整页刷新、站内路由切换、点开商品详情开的新标签——都会弹登录框，它的遮罩会拦截点击，表现为元素「看得见点不到」。`scripts/xy-auto-close.mjs` 把关闭逻辑挂到每个新文档上，并能在关不掉时直接隐藏弹窗容器。

## 前置条件

- 依赖 ego-browser skill；浏览器必须在前台可见，调研结束再关闭空间。
- 全程免登录，不输入账号、密码、验证码。遇到人机验证、风控拦截或要求身份验证时，立即停下并把情况告诉用户，等用户处理，不要换 IP、换 UA、换镜像或重试绕过。
- 脚本通过 `ego-browser nodejs -e` 执行：`-e` 里的代码只能用双引号，路径要写绝对路径（ego-browser 的子进程不继承环境变量，工作目录是 `/`，所以别依赖 `process.env` 和相对路径）。

## 快速开始

把 `scripts/xy-search.mjs` 按下面的方式导入即可，`<skill>` 指本 skill 所在目录（即 `SKILL.md` 的所在目录）：

```bash
ego-browser nodejs -e '
const { searchXianyu } = await import("<skill>/scripts/xy-search.mjs");
await searchXianyu({ query: "多邻国会员", limit: 30 });
'
```

常用参数：

| 参数 | 说明 |
| --- | --- |
| `query` / `queries` | 单个关键词，或一组关键词（一次浏览器往返跑完，推荐用 `queries` 做多词对比） |
| `limit` | 每个关键词取多少条，默认 30 |
| `details` | 顺带打开前 N 条商品详情，默认 0 |
| `detailDir` | 配上 `details` 时，把每条的封面图存到这个目录 |
| `spaceId` | 复用已有的调研空间（上一次调用返回的 `spaceId`） |

## 输出

每次调用打印并返回同一份 JSON：

```json
{
  "spaceId": 3,
  "page": "p1",
  "results": [
    {
      "query": "多邻国会员",
      "ok": true,
      "url": "https://www.goofish.com/search?q=...",
      "count": 30,
      "items": [
        { "id": "1087315850942", "title": "...", "price": 9.9, "want": 2808, "url": "...", "image": "...", "text": "卡片原文" }
      ],
      "details": [
        { "index": 0, "id": "...", "title": "...", "price": "3.99", "wants": "1555", "views": "7429",
          "seller": "...", "sellerMeta": "杭州 来闲鱼5年 卖出422573件宝贝 好评率99%", "desc": "详情文案", "images": ["..."], "coverImage": "/abs/path.jpg" }
      ]
    }
  ]
}
```

`ok: false` 时看 `reason`：多数情况是没有走搜索框，或页面还没加载完。

## 调研时的经验

- 一条关键词通常不够。同一批商品会在不同标题里换词，用 `queries: ["多邻国", "多邻国会员", "多邻国年卡", "duolingo"]` 一次跑完，再合并去重。
- 搜索结果卡片里的 `text` 已经带标题、价格、想要人数、卖家地区和信用，很多时候不用点进详情就能得出结论。
- 需要看主图风格、购买须知、发货话术时再开 `details`。详情页的 `desc` 是完整正文，`images[0]` 是主图原图（已去掉 CDN 缩略参数）。
- 同一个空间可以反复调用：第二次调用传入上次的 `spaceId`，就不用重新开浏览器。

## 收尾

调研结束、已把结论交付给用户后，关闭浏览器空间：

```bash
ego-browser nodejs -e '
const { finishXianyu } = await import("<skill>/scripts/xy-search.mjs");
const space = await taskSpace(<spaceId>);
await finishXianyu(space);
'
```

只有用户明确要求保留页面时才不关。

## 出问题时

先读 [references/troubleshooting.md](references/troubleshooting.md)：里面列了弹窗关不掉、结果为空、点击被拦截、详情页新标签没装上脚本等情况的判定和处置方式。
