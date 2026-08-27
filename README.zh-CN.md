# Review Stars

Review Stars 把 GitHub Stars 变成可执行的整理清单：同步 Star 仓库，通过 OpenAI 兼容接口判断哪些 Star 可能已经不值得保留并给出原因，然后通过 Telegram 定时发送随机仓库回顾提醒。

前端使用 Vue 3 + Vite，后端使用 Go。生产构建会把 `web/dist` 嵌入 Go 二进制，因此部署时只需要一个可执行文件。仓库元数据和评审结果保存在 SQLite 中。

English documentation: [README.md](README.md)

## 快速开始

需要 Go 1.22+、Node.js 20+ 和 npm。

```bash
cp .env.example .env
# 编辑 .env，至少配置 GITHUB_TOKEN 和 AI_API_KEY

cd web
npm install
npm run build
cd ..

go run .
```

程序会自动读取项目根目录的 `.env`；如果 shell 中已经导出了同名变量，shell 变量优先。

打开 <http://localhost:8080>。

程序启动时只从 SQLite 加载已有数据，不会自动同步 GitHub。首次启动或需要刷新 Star 列表时，点击“同步仓库”；它会加载全部 Stars，并复用仓库元数据未变化的 review。

也可以直接构建单文件：

```bash
make build
./review-stars
```

## 配置

- `GITHUB_TOKEN`：GitHub fine-grained PAT。需要 `Starring: read`；如果要使用页面里的“取消 Star”，还需要 `Starring: read and write`。
- `AI_API_KEY`：配置的 OpenAI 兼容 AI 服务的 API key。
- `AI_BASE_URL`：服务商的 base URL，程序会自动追加 `/chat/completions`。DeepSeek 示例为 `https://api.deepseek.com`。
- `AI_MODEL`：服务商支持的模型名，例如 `deepseek-v4-flash`。
- `AI_THINKING`：DeepSeek 思考模式，默认 `disabled`；复杂评审时可设置为 `enabled`。
- `AI_MAX_TOKENS`：每个 AI 批次响应允许生成的最大 token 数，默认 `16000`。
- `APP_LANGUAGE`：页面和评审使用的语言，默认 `zh-CN`，也可以设置为 `en`。
- `TELEGRAM_BOT_TOKEN`、`TELEGRAM_CHAT_ID`：两者都配置后启用 cron Telegram 后台提醒。
- `DATABASE_FILE`：SQLite 数据库路径，默认 `review-stars.db`。
- `REVIEW_BATCH_SIZE`：每次 AI 请求的仓库数，默认 50。程序会加载全部 Stars，再按批次请求 AI。
- `REVIEW_CRON`：可选的 5 段 cron 表达式，也支持 `@every 24h`。配置后从 SQLite 随机抽取仓库发送 Telegram 回顾，不会自动同步 GitHub。
- `REVIEW_COUNT`：每次 cron 回顾发送的仓库数，默认 1。
- `RULE_STATUSES`：AI 前置状态筛选，逗号分隔，默认 `archived`，例如 `archived,disabled`。
- `RULE_STALE_DAYS`：前置筛选判定长期未更新的天数，默认 180；设置为 `0` 关闭。
- `RULE_MAX_STARS`：前置筛选判定 Star 不足的上限，默认 1000；设置为 `0` 关闭。
- `HOST`：监听地址，默认 `127.0.0.1`。
- `PORT`：HTTP 端口，默认 `8080`。

`REVIEW_MAX_REPOS` 已不再使用。同步和评审范围都是全部 Stars；`REVIEW_BATCH_SIZE` 只控制单次请求的仓库数，`REVIEW_COUNT` 控制定时 Telegram 回顾数量。

旧的 `OPENROUTER_*` 和 `APP_URL` 配置不再读取，请改用 `AI_API_KEY`、`AI_BASE_URL` 和 `AI_MODEL`。

## Telegram 设置

1. 在 Telegram 联系 `@BotFather`，使用 `/newbot` 创建机器人，得到 `TELEGRAM_BOT_TOKEN`。
2. 直接给机器人发送一条消息，不需要群组。
3. 访问 `https://api.telegram.org/bot<TOKEN>/getUpdates`，从返回 JSON 中找到 `message.chat.id`。
4. 将这个值填入 `TELEGRAM_CHAT_ID`，重启应用。

Telegram 只由后端 cron 任务使用，页面上没有手动 Telegram 按钮。

页面默认跟随操作系统亮/暗色偏好，也可以点击顶部太阳/月亮按钮手动切换；选择会保存在浏览器中。

## API

```text
GET    /api/health
GET    /api/stars             # 读取 SQLite 中已同步的仓库
POST   /api/sync              # 从 GitHub 同步全部 Stars
GET    /api/review
POST   /api/review             # AI 分批评审；默认跳过已有结果；?force=1 显式强制全部重评
GET    /api/random
DELETE /api/stars/:owner/:repo
```

## 外部请求日志

GitHub、AI 和 Telegram 请求都会输出请求、状态码、耗时、响应字节数以及完整响应体。每次 AI 评审还会汇总输出 prompt、completion、total 和 prompt-cache-hit token：

```text
[github] response method=GET url=https://api.github.com/user/starred?... status=200 duration=320ms bytes=...
[ai] response method=POST url=https://api.deepseek.com/chat/completions status=200 duration=8.4s bytes=...
[telegram] response method=POST url=https://api.telegram.org/bot-REDACTED/sendMessage status=200 duration=210ms bytes=...
```

日志不会记录 Authorization 请求头，Telegram Bot Token 会脱敏。GitHub 和 AI 响应预览可能包含仓库或 prompt 数据，生产环境请按需限制日志输出。

## 评审行为

手动同步会将全部 GitHub Stars、仓库元数据和 AI 评审缓存保存到 SQLite，最终清单会合并当前规则命中结果与已有 AI 结果。仓库指纹只使用会影响评审的字段：描述前 200 个字符、语言、前 10 个 topics、归档/禁用/fork 状态，以及按月份分桶的最后推送时间；Star、fork、issue 数量、更新时间和收藏时间变化不会使 AI review 失效。每次 AI 评审前，所有启用的确定性规则会同时执行；全部命中的仓库直接标记为建议取消，不消耗 AI，只有剩余仓库才进入 AI。指纹未变化时会复用 AI review，只对新仓库或发生变化的仓库分批请求 AI。页面主 AI 评审操作也会复用未变化的结果；只有 API 显式传入 `?force=1` 才会全部重评。“继续 AI 评审”会跳过已有 AI 结果。

发送给 AI 的数据只保留决策相关元数据：description 最多 200 个字符，topics 最多 10 个，去掉仓库 URL、fork 数和 open issue 数。AI 返回最多 2 条简短原因，并删除未在页面使用的 `next_action` 字段。

规则在 AI 之前执行，不是 AI 兜底。启用的规则组必须同时命中：默认要求仓库已归档、180 天未更新且 Star 少于 1000；`RULE_STATUSES` 内的多个状态是 OR 关系。如果筛选后仍有仓库而 AI 接口不可用，统一评审会直接返回错误。

随机回顾区域可以填写数量，在可用范围内一次抽取多个仓库。归档仓库不会调用取消 Star 接口，而是打开当前用户的 GitHub Stars 搜索页面，需要在那里手动取消；未归档仓库仍使用 GitHub API，并且需要写权限。

修改 `APP_LANGUAGE` 后，当前已加载的其他语言评审结果会被视为无效，请重新运行统一评审。旧 review 数据不会迁移。

旧的 `.review-cache.json` 文件不再读取，请使用当前应用创建的 SQLite 数据库。

这是面向单用户/本机运行的 MVP，API 当前没有额外登录层。不要把它直接暴露到公网；多人使用前应增加 OAuth、会话和权限控制。

## 设计取舍

规则筛选使用确定性的本地条件，在 AI 之前执行且不调用 AI；命中的仓库会和 AI 结果一起展示。未命中的仓库使用配置的 OpenAI 兼容 Chat Completions 接口，因此服务商和模型都由配置决定，而不是写死在程序中。
