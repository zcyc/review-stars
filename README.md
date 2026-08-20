# Review Stars

一个把 GitHub Stars 重新变成“可行动收藏夹”的小工具：同步你的 Star，使用 OpenRouter 免费模型给出建议取消 Star 的仓库和原因，随机抽取一个仓库进行回顾，并通过 Telegram 机器人提醒。

前端使用 Vue 3 + Vite，后端使用 Go。生产构建会把 `web/dist` 嵌入 Go 二进制，因此运行时只需要一个文件；仓库和 review 结果保存在 SQLite 文件中。

## 快速开始

需要 Go 1.22+、Node.js 20+ 和 npm。

```bash
cp .env.example .env
# 编辑 .env，至少配置 GITHUB_TOKEN 和 OPENROUTER_API_KEY

cd web
npm install
npm run build
cd ..

go run .
```

程序启动时会自动读取项目根目录的 `.env`；如果 shell 中已经导出了同名变量，shell 变量优先。

打开 <http://localhost:8080>。

程序启动时只从 SQLite 加载已有数据，不会自动同步 GitHub。首次启动或需要刷新 Star 列表时，点击页面上的“同步仓库”按钮；它会加载全部 Stars，并复用元数据未变化的 review。

也可以直接构建单文件：

```bash
make build
./review-stars
```

## 配置

- `GITHUB_TOKEN`：GitHub fine-grained PAT。需要 `Starring: read`；如果要使用页面里的“取消 Star”，需要 `Starring: read and write`。
- `OPENROUTER_API_KEY`：OpenRouter API key。默认使用 `openrouter/free` 免费模型路由。
- `OPENROUTER_MODEL`：可替换为任意可用模型，默认 `openrouter/free`。
- `TELEGRAM_BOT_TOKEN`、`TELEGRAM_CHAT_ID`：配置后启用 Telegram 提醒；不配置时其余功能仍可使用。
- `DATABASE_FILE`：SQLite 数据库路径，默认 `review-stars.db`。保存全部同步到的仓库和 review 结果。
- `REVIEW_BATCH_SIZE`：每次发送给 AI 的仓库数，默认 20。程序会加载全部 Stars，再按批次请求 AI。
- `REVIEW_CRON`：可选的 5 段 cron 表达式，也支持 `@every 24h`。配置后按计划从 SQLite 中随机抽取仓库发送 Telegram 回顾；不会因为定时任务自动请求 GitHub。
- `REVIEW_COUNT`：每次 cron 回顾发送的仓库数，默认 1；同一次执行内不会重复抽取。
- `RULE_STATUSES`：规则评审关注的状态，逗号分隔，默认 `archived`；可配置为 `archived,disabled`。
- `RULE_STALE_DAYS`：规则评审判定“长期未更新”的天数，默认 180；设置为 `0` 关闭。
- `RULE_MAX_STARS`：规则评审判定 Star 不足的上限，默认 1000；设置为 `0` 关闭。
- `PORT`：HTTP 端口，默认 8080。
- `HOST`：监听地址，默认 `127.0.0.1`（个人本机使用更安全）；部署到容器或局域网时可设为 `0.0.0.0`。
- `APP_URL`：可选，用于 OpenRouter 的 `HTTP-Referer` 和 `X-Title` 归因头。

## Telegram 设置

1. 在 Telegram 联系 `@BotFather`，使用 `/newbot` 创建机器人，得到 `TELEGRAM_BOT_TOKEN`。
2. 给机器人发送一条消息。
3. 访问 `https://api.telegram.org/bot<TOKEN>/getUpdates`，从返回 JSON 中找到 `message.chat.id`。
4. 将这个值填入 `TELEGRAM_CHAT_ID`，重启应用。

## API

```text
GET    /api/health
GET    /api/stars             # 读取 SQLite 中已同步的仓库
POST   /api/sync              # 从 GitHub 同步全部 Stars
GET    /api/review
POST   /api/review             # 仅 AI 分批评审；默认跳过已有；?force=1 强制全部重评
GET    /api/rule-review
POST   /api/rule-review        # 按 RULE_* 配置执行规则评审
GET    /api/random
POST   /api/remind
DELETE /api/stars/:owner/:repo
```

## 外部请求日志

GitHub、OpenRouter 和 Telegram 请求都会输出响应日志，包含状态码、耗时、响应字节数和最多 2000 字节的响应体预览。例如：

```text
[github] response method=GET url=https://api.github.com/user/starred?... status=200 duration=320ms bytes=...
[openrouter] response method=POST url=https://openrouter.ai/api/v1/chat/completions status=200 duration=8.4s bytes=...
[telegram] response method=POST url=https://api.telegram.org/bot-REDACTED/sendMessage status=200 duration=210ms bytes=...
```

日志不会记录 Authorization 请求头，Telegram Bot Token 会脱敏；GitHub 响应可能包含仓库元数据，生产环境请按需限制日志输出。

程序会在手动同步时加载全部 GitHub Stars，并将仓库数据、AI review 和规则 review 分别持久化到 SQLite。重复 AI 评审时会复用元数据未变化的 AI 结果，只对新仓库或发生变化的仓库分批请求 AI。规则评审会按照当前配置重新计算。GitHub 数据本身不会被修改，除非用户明确点击“取消 Star”。

`REVIEW_MAX_REPOS` 已不再使用，也不需要配置。现在的边界分别是：`REVIEW_BATCH_SIZE` 控制单次 AI 请求包含多少仓库，`REVIEW_COUNT` 控制定时 Telegram 回顾发送多少仓库；同步和评审范围默认都是全部 Stars。

AI 评审不再使用规则兜底。OpenRouter 不可用时，AI 接口会直接返回错误；需要离线或低成本筛选时，单独运行“规则评审”。规则评审默认列出同时满足“已归档、180 天未更新、Star 少于 1000”的仓库，三类条件可以分别配置。启用的条件组之间是 AND 关系，`RULE_STATUSES` 内的多个状态是 OR 关系。

这是面向单用户/本机运行的 MVP，API 当前没有额外登录层。不要把它直接暴露到公网；如果需要多人使用，应在前面增加 OAuth、会话和权限控制。

## 设计取舍

规则评审使用确定性的本地条件，不调用 AI；AI 评审则只接受 OpenRouter 返回的完整结果，不会把规则结果伪装成 AI 结果。
