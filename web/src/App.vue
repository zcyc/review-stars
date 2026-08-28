<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

const messages = {
  'zh-CN': {
    connected: 'GitHub 已连接',
    waitingGithub: '等待 GitHub 配置',
    reposLoaded: '个仓库已加载',
    noReposLoaded: '尚未加载仓库',
    sync: '↻ 同步仓库', syncing: '同步中…', startAI: '开始 AI 评审', rerunAI: '更新 AI 评审',
    viewingAI: 'AI 正在查看…', continueAI: '继续 AI 评审（跳过已有）', waitingAI: '等待 AI…', aiCompleteButton: 'AI 评审已完成', modelSuffix: '· OpenAI 兼容接口',
    syncNotice: '已从 GitHub 同步 {{count}} 个仓库，已有评审已复用',
    aiNotice: '已完成 {{total}} 个仓库：复用已有评审 {{cached}} 个，AI 本次评审 {{reviewed}} 个',
    continueNotice: '继续 AI 评审完成：跳过已有 {{cached}} 个，本次新增评审 {{reviewed}} 个',
    statCollected: '已收藏仓库', statCollectedFoot: '同步自 GitHub', statUnstar: '建议取消 Star', statUnstarAI: 'AI 与规则建议',
    statReview: '需要人工回顾', statReviewFoot: '值得打开看一眼', statKeep: '建议保留', statKeepFoot: '仍然值得放在收藏夹',
    randomTitle: '随机回顾仓库', randomEmpty: '点击按钮抽取你的回顾对象', randomCount: '数量', draw: '抽取',
    repoListTitle: '仓库清单',
    searchPlaceholder: '搜索仓库…', all: '全部状态', unstarFilter: '建议取消', reviewFilter: '需要回顾', keepFilter: '建议保留',
    loadingStars: '正在读取本地仓库数据…', noStarsTitle: '还没有同步仓库', noStarsText: '点击上方“同步仓库”从 GitHub 获取全部 Stars。', noMatches: '没有匹配的仓库', noReview: '还没有评审结果', noMatchesText: '试试换一个搜索词或状态筛选。', startReviewHint: '点击上方“开始 AI 评审”。',
    noDescription: '这个仓库没有提供描述。', archived: '已归档', active: '未归档', fork: 'FORK', starredAt: '收藏于 {{date}}', openRepoTitle: '打开仓库', openStars: '打开 GitHub Stars', unstarAction: '取消 Star', findArchived: '手动取消 Star',
    decisionUnstar: '建议取消', decisionReview: '需要回顾', decisionKeep: '建议保留', pendingDecision: '待评审', confirmUnstar: '确定取消 {{repo}} 的 Star 吗？此操作会直接修改 GitHub。', archivedOpenStars: '{{repo}} 已归档，已打开 GitHub Stars 搜索，请在那里手动取消 Star。', unstarDone: '已取消 {{repo}} 的 Star',
    unknownDate: '未知', unknownUpdated: '更新时间未知', recentlyUpdated: '最近更新', todayActive: '今天有活动', daysAgo: '{{count}} 天前更新', monthsAgo: '{{count}} 个月前更新', yearsAgo: '{{count}} 年前更新', requestFailed: '请求失败（{{status}}）', switchToLight: '切换到亮色模式', switchToDark: '切换到暗色模式',
  },
  en: {
    connected: 'GitHub connected', waitingGithub: 'Waiting for GitHub configuration', reposLoaded: 'repositories loaded', noReposLoaded: 'No repositories loaded',
    sync: '↻ Sync repositories', syncing: 'Syncing…', startAI: 'Start AI review', rerunAI: 'Update AI review', viewingAI: 'AI is reviewing…', continueAI: 'Continue AI review (skip existing)', waitingAI: 'Waiting for AI…', aiCompleteButton: 'AI review complete', modelSuffix: '· OpenAI-compatible API',
    syncNotice: 'Synced {{count}} repositories from GitHub; existing reviews were reused', aiNotice: 'Finished {{total}} repositories: {{rules}} matched rules, reused {{cached}} reviews, and added {{reviewed}} AI reviews', continueNotice: 'AI review continued: {{rules}} matched rules, skipped {{cached}} existing reviews, and added {{reviewed}} new reviews',
    statCollected: 'Starred repositories', statCollectedFoot: 'Synced from GitHub', statUnstar: 'Suggested to unstar', statUnstarAI: 'AI and rule recommendations', statReview: 'Needs a human look', statReviewFoot: 'Worth opening once', statKeep: 'Suggested to keep', statKeepFoot: 'Still useful in your collection',
    randomTitle: 'Random repository recall', randomEmpty: 'Pick your next repositories to revisit', randomCount: 'Count', draw: 'Pick', repoListTitle: 'Repository list',
    searchPlaceholder: 'Search repositories…', all: 'All statuses', unstarFilter: 'Suggested to unstar', reviewFilter: 'Needs review', keepFilter: 'Suggested to keep', loadingStars: 'Reading local repository data…', noStarsTitle: 'No repositories synced', noStarsText: 'Click “Sync repositories” above to load all of your GitHub Stars.', noMatches: 'No matching repositories', noReview: 'No review results yet', noMatchesText: 'Try another search term or status filter.', startReviewHint: 'Use “Start AI review” above.',
    noDescription: 'This repository has no description.', archived: 'ARCHIVED', active: 'ACTIVE', fork: 'FORK', starredAt: 'Starred {{date}}', openRepoTitle: 'Open repository', openStars: 'Open GitHub Stars', unstarAction: 'Unstar', findArchived: 'Remove Star manually', decisionUnstar: 'Suggested to unstar', decisionReview: 'Needs review', decisionKeep: 'Suggested to keep', pendingDecision: 'Pending review', confirmUnstar: 'Unstar {{repo}}? This will modify GitHub directly.', archivedOpenStars: '{{repo}} is archived. GitHub Stars search was opened; remove the Star there manually.', unstarDone: 'Unstarred {{repo}}',
    unknownDate: 'Unknown', unknownUpdated: 'Update date unknown', recentlyUpdated: 'Recently updated', todayActive: 'Active today', daysAgo: 'Updated {{count}} days ago', monthsAgo: 'Updated {{count}} months ago', yearsAgo: 'Updated {{count}} years ago', requestFailed: 'Request failed ({{status}})', switchToLight: 'Switch to light mode', switchToDark: 'Switch to dark mode',
  },
}

const language = ref('zh-CN')
const themeMode = ref('system')
const systemDark = ref(false)
let colorSchemeMedia

function t(key, variables = {}) {
  const dictionary = messages[language.value] || messages['zh-CN']
  const template = dictionary[key] || messages['zh-CN'][key] || key
  return Object.entries(variables).reduce((result, [name, value]) => result.replaceAll(`{{${name}}}`, String(value)), template)
}

const isDarkTheme = computed(() => themeMode.value === 'dark' || (themeMode.value === 'system' && systemDark.value))

function applyTheme() {
  const root = document.documentElement
  root.classList.toggle('theme-light', themeMode.value === 'light')
  root.classList.toggle('theme-dark', themeMode.value === 'dark')
  root.style.colorScheme = themeMode.value === 'system' ? 'light dark' : themeMode.value
  const colorSchemeMeta = document.querySelector('meta[name="color-scheme"]')
  if (colorSchemeMeta) colorSchemeMeta.content = themeMode.value === 'system' ? 'light dark' : themeMode.value
  const themeColorMeta = document.querySelector('meta[name="theme-color"]')
  if (themeColorMeta) themeColorMeta.content = isDarkTheme.value ? '#0d1117' : '#f6f8fa'
}

function handleSystemThemeChange(event) {
  systemDark.value = event.matches
  if (themeMode.value === 'system') applyTheme()
}

function initTheme() {
  const saved = window.localStorage.getItem('review-stars-theme')
  themeMode.value = saved === 'light' || saved === 'dark' ? saved : 'system'
  colorSchemeMedia = window.matchMedia('(prefers-color-scheme: dark)')
  systemDark.value = colorSchemeMedia.matches
  colorSchemeMedia.addEventListener('change', handleSystemThemeChange)
  applyTheme()
}

function toggleTheme() {
  const next = isDarkTheme.value ? 'light' : 'dark'
  themeMode.value = next
  window.localStorage.setItem('review-stars-theme', next)
  applyTheme()
}

const stars = ref([])
const reviews = ref([])
const stats = ref({ total: 0, unstar: 0, review: 0, keep: 0 })
const health = ref(null)
const aiComplete = ref(false)
const randomRepos = ref([])
const randomCount = ref(1)
const loadingStars = ref(false)
const syncing = ref(false)
const reviewing = ref(false)
const busyRepo = ref('')
const error = ref('')
const errorLink = ref('')
const warning = ref('')
const notice = ref('')
const query = ref('')
const filter = ref('all')

const visibleReviews = computed(() => {
  const reviewByName = new Map(reviews.value.map(review => [review.full_name, review]))
  const source = stars.value.length
    ? stars.value.map(repo => reviewByName.get(repo.full_name) || {
        ...repo,
        decision: 'review',
        score: 0,
        summary: t('noReview'),
        reasons: [t('startReviewHint')],
        source: 'pending',
      })
    : reviews.value
  const needle = query.value.trim().toLowerCase()
  return source.filter(repo => {
    const matchesQuery = !needle || [repo.full_name, repo.description, repo.language].filter(Boolean).some(value => value.toLowerCase().includes(needle))
    const matchesFilter = filter.value === 'all' || repo.decision === filter.value
    return matchesQuery && matchesFilter
  })
})

const hasReviews = computed(() => reviews.value.length > 0)

async function request(path, options = {}) {
  const response = await fetch(path, { headers: { 'Content-Type': 'application/json', ...(options.headers || {}) }, ...options })
  const body = await response.json().catch(() => ({}))
  if (!response.ok) {
    const requestError = new Error(body.error || t('requestFailed', { status: response.status }))
    requestError.status = response.status
    requestError.body = body
    throw requestError
  }
  return body
}

async function loadHealth() {
  try {
    const data = await request('/api/health')
    health.value = data
    language.value = data.language || 'zh-CN'
    document.documentElement.lang = language.value === 'en' ? 'en' : 'zh-CN'
  } catch (err) {
    errorLink.value = ''
    error.value = err.message
  }
}

async function loadStars() {
  loadingStars.value = true
  error.value = ''
  try {
    const data = await request('/api/stars')
    stars.value = data.repositories || []
  } catch (err) {
    errorLink.value = ''
    error.value = err.message
  } finally {
    loadingStars.value = false
  }
}

async function syncStars() {
  syncing.value = true
  error.value = ''
  errorLink.value = ''
  warning.value = ''
  notice.value = ''
  try {
    const data = await request('/api/sync', { method: 'POST' })
    stars.value = data.repositories || []
    reviews.value = data.reviews || []
    stats.value = data.stats || stats.value
    aiComplete.value = Boolean(data.ai_complete)
    notice.value = t('syncNotice', { count: stars.value.length })
  } catch (err) {
    errorLink.value = ''
    error.value = err.message
  } finally {
    syncing.value = false
  }
}

async function loadExistingReview() {
  try {
    const data = await request('/api/review')
    reviews.value = data.reviews || []
    stats.value = data.stats || stats.value
    aiComplete.value = Boolean(data.ai_complete)
  } catch {
    // A first visit has no review yet; the empty state is intentional.
  }
}

async function runAIReview(continuing = false) {
  reviewing.value = true
  error.value = ''
  errorLink.value = ''
  warning.value = ''
  notice.value = ''
  try {
    const queryString = continuing ? '?continue=1' : ''
    const data = await request(`/api/review${queryString}`, { method: 'POST' })
    reviews.value = data.reviews || []
    stats.value = data.stats || stats.value
    aiComplete.value = Boolean(data.ai_complete)
    warning.value = data.warning || ''
    notice.value = continuing
      ? t('continueNotice', { rules: data.rule_matched_count || 0, cached: data.cached_count || 0, reviewed: data.ai_reviewed_count || 0 })
      : t('aiNotice', { total: stats.value.total, rules: data.rule_matched_count || 0, cached: data.cached_count || 0, reviewed: data.ai_reviewed_count || 0 })
  } catch (err) {
    errorLink.value = ''
    error.value = err.message
  } finally {
    reviewing.value = false
  }
}

async function runReview() { await runAIReview(false) }
async function continueReview() {
  if (aiComplete.value) return
  await runAIReview(true)
}

async function pickRandom() {
  error.value = ''
  errorLink.value = ''
  try {
    const count = Math.max(1, Number(randomCount.value) || 1)
    randomCount.value = count
    const data = await request(`/api/random?count=${encodeURIComponent(count)}`)
    randomRepos.value = data.repositories || []
  } catch (err) {
    errorLink.value = ''
    error.value = err.message
  }
}

async function unstar(repo) {
  error.value = ''
  errorLink.value = ''
  notice.value = ''
  if (repo.archived) {
    const target = `https://github.com/stars?q=${encodeURIComponent(repo.full_name)}`
    window.open(target, '_blank', 'noopener,noreferrer')
    notice.value = t('archivedOpenStars', { repo: repo.full_name })
    return
  }
  if (!window.confirm(t('confirmUnstar', { repo: repo.full_name }))) return
  busyRepo.value = repo.full_name
  try {
    await request(`/api/stars/${encodeURIComponent(repo.full_name)}`, { method: 'DELETE' })
    stars.value = stars.value.filter(item => item.full_name !== repo.full_name)
    const aiRepo = reviews.value.find(item => item.full_name === repo.full_name)
    reviews.value = reviews.value.filter(item => item.full_name !== repo.full_name)
    const decrement = (current, removed) => removed ? { ...current, total: Math.max(0, current.total - 1), unstar: Math.max(0, current.unstar - (removed.decision === 'unstar' ? 1 : 0)) } : current
    stats.value = decrement(stats.value, aiRepo)
    notice.value = t('unstarDone', { repo: repo.full_name })
  } catch (err) {
    error.value = err.message
    if (err.status === 403 && err.body?.stars_url) {
      errorLink.value = err.body.stars_url
    }
  } finally {
    busyRepo.value = ''
  }
}

function decisionLabel(decision) { return { unstar: t('decisionUnstar'), review: t('decisionReview'), keep: t('decisionKeep') }[decision] || t('pendingDecision') }
function decisionClass(decision) { return `decision-${decision || 'review'}` }
function formatNumber(value) { return new Intl.NumberFormat(language.value === 'en' ? 'en-US' : 'zh-CN').format(value || 0) }

function formatDate(date) {
  if (!date) return t('unknownDate')
  const value = new Date(date)
  if (Number.isNaN(value.getTime())) return t('unknownDate')
  return new Intl.DateTimeFormat(language.value === 'en' ? 'en-US' : 'zh-CN', { year: 'numeric', month: 'short', day: 'numeric' }).format(value)
}

function relativeDate(date) {
  if (!date) return t('unknownDate')
  const value = new Date(date)
  if (Number.isNaN(value.getTime()) || value.getUTCFullYear() < 2000) return t('unknownUpdated')
  const days = Math.floor((Date.now() - value.getTime()) / 86400000)
  if (days < 0) return t('recentlyUpdated')
  if (days < 1) return t('todayActive')
  if (days < 30) return t('daysAgo', { count: days })
  if (days < 365) return t('monthsAgo', { count: Math.floor(days / 30) })
  return t('yearsAgo', { count: Math.floor(days / 365) })
}

onMounted(async () => {
  initTheme()
  await Promise.all([loadHealth(), loadStars(), loadExistingReview()])
})

onBeforeUnmount(() => colorSchemeMedia?.removeEventListener('change', handleSystemThemeChange))
</script>

<template>
  <div class="app-shell">
    <header class="topbar">
      <a class="brand" href="/"><span class="brand-star" aria-hidden="true"><svg viewBox="0 0 24 24" focusable="false"><path d="m12.672.668 3.059 6.197 6.838.993a.75.75 0 0 1 .416 1.28l-4.948 4.823 1.168 6.812a.75.75 0 0 1-1.088.79L12 18.347l-6.116 3.216a.75.75 0 0 1-1.088-.791l1.168-6.811-4.948-4.823a.749.749 0 0 1 .416-1.279l6.838-.994L11.327.668a.75.75 0 0 1 1.345 0Z"/></svg></span><span class="brand-label">Review</span><span class="brand-ai">Stars</span></a>
      <div class="topbar-end">
        <button class="theme-toggle" :aria-label="t(isDarkTheme ? 'switchToLight' : 'switchToDark')" :title="t(isDarkTheme ? 'switchToLight' : 'switchToDark')" @click="toggleTheme">{{ isDarkTheme ? '☀' : '☾' }}</button>
        <div class="topbar-meta"><span class="live-dot" :class="{ offline: health && !health.github_configured }"></span>{{ health?.github_configured ? t('connected') : t('waitingGithub') }}</div>
      </div>
    </header>

    <main class="page-wrap">
      <section class="action-bar">
        <div class="action-bar-meta"><strong>{{ stars.length ? `${formatNumber(stars.length)} ${t('reposLoaded')}` : t('noReposLoaded') }}</strong><span class="model-note">{{ health?.ai_model || 'deepseek-v4-flash' }} {{ t('modelSuffix') }}</span></div>
        <div class="action-bar-actions">
          <button class="button button-outline" :disabled="syncing || !health?.github_configured" @click="syncStars"><span v-if="syncing" class="spinner"></span><span>{{ syncing ? t('syncing') : t('sync') }}</span></button>
          <button class="button button-primary" :disabled="reviewing || !health?.github_configured || !health?.ai_configured || !stars.length" @click="runReview"><span v-if="reviewing" class="spinner"></span><span>{{ reviewing ? t('viewingAI') : (hasReviews ? t('rerunAI') : t('startAI')) }}</span></button>
          <button class="button button-outline" :disabled="reviewing || aiComplete || !health?.github_configured || !health?.ai_configured || !stars.length" @click="continueReview"><span>{{ aiComplete ? t('aiCompleteButton') : (reviewing ? t('waitingAI') : t('continueAI')) }}</span></button>
        </div>
      </section>

      <div v-if="error" class="alert alert-error"><span>!</span>{{ error }}<a v-if="errorLink" :href="errorLink" target="_blank" rel="noreferrer">{{ t('openStars') }}</a></div><div v-if="warning" class="alert alert-warning"><span>△</span>{{ warning }}</div><div v-if="notice" class="alert alert-success"><span>✓</span>{{ notice }}</div>

      <section class="stats-grid">
        <article class="stat-card stat-total"><div class="stat-label">{{ t('statCollected') }}</div><div class="stat-value">{{ stats.total || stars.length }}</div><div class="stat-foot">{{ t('statCollectedFoot') }}</div></article>
        <article class="stat-card stat-danger"><div class="stat-label">{{ t('statUnstar') }}</div><div class="stat-value">{{ stats.unstar }}</div><div class="stat-foot">{{ t('statUnstarAI') }}</div></article>
        <article class="stat-card stat-warm"><div class="stat-label">{{ t('statReview') }}</div><div class="stat-value">{{ stats.review }}</div><div class="stat-foot">{{ t('statReviewFoot') }}</div></article>
        <article class="stat-card stat-green"><div class="stat-label">{{ t('statKeep') }}</div><div class="stat-value">{{ stats.keep }}</div><div class="stat-foot">{{ t('statKeepFoot') }}</div></article>
      </section>

      <section class="focus-grid">
        <article class="random-card">
          <div class="random-header"><h2>{{ t('randomTitle') }}</h2><div class="random-actions"><label class="random-count"><span>{{ t('randomCount') }}</span><input v-model.number="randomCount" type="number" min="1" /></label><button class="button button-dark" @click="pickRandom">{{ t('draw') }}</button></div></div>
          <div v-if="randomRepos.length" class="random-results"><div v-for="repo in randomRepos" :key="repo.full_name" class="random-result"><div class="repo-avatar">{{ repo.name?.slice(0, 1).toUpperCase() }}</div><div class="random-info"><a :href="repo.html_url" target="_blank" rel="noreferrer">{{ repo.full_name }}</a><span>{{ repo.summary || repo.description || t('noDescription') }}</span></div><span class="decision-pill" :class="decisionClass(repo.decision)">{{ decisionLabel(repo.decision) }}</span></div></div>
          <div v-else class="random-placeholder"><span>✦</span><span>{{ t('randomEmpty') }}</span></div>
        </article>
      </section>

      <section class="review-section">
        <div class="section-heading"><h2>{{ t('repoListTitle') }}</h2><div class="list-tools"><label class="search-box"><span>⌕</span><input v-model="query" :placeholder="t('searchPlaceholder')" /></label><select v-model="filter"><option value="all">{{ t('all') }}</option><option value="unstar">{{ t('unstarFilter') }}</option><option value="review">{{ t('reviewFilter') }}</option><option value="keep">{{ t('keepFilter') }}</option></select></div></div>
        <div v-if="loadingStars" class="empty-state"><span class="spinner spinner-dark"></span><p>{{ t('loadingStars') }}</p></div>
        <div v-else-if="!stars.length" class="empty-state"><div class="empty-icon">☆</div><h3>{{ t('noStarsTitle') }}</h3><p>{{ t('noStarsText') }}</p></div>
        <div v-else-if="!visibleReviews.length" class="empty-state"><div class="empty-icon">⌕</div><h3>{{ reviews.length ? t('noMatches') : t('noReview') }}</h3><p>{{ reviews.length ? t('noMatchesText') : t('startReviewHint') }}</p></div>
        <div v-else class="repo-list"><article v-for="repo in visibleReviews" :key="repo.full_name" class="repo-row"><div class="repo-main"><div class="repo-avatar repo-avatar-large">{{ repo.name?.slice(0, 1).toUpperCase() }}</div><div class="repo-copy"><div class="repo-title-line"><a :href="repo.html_url" target="_blank" rel="noreferrer">{{ repo.full_name }}</a><span v-if="repo.archived" class="tiny-tag">{{ t('archived') }}</span><span v-if="repo.fork" class="tiny-tag">{{ t('fork') }}</span></div><p>{{ repo.description || t('noDescription') }}</p><div class="repo-meta"><span v-if="repo.language"><i class="language-dot"></i>{{ repo.language }}</span><span>★ {{ formatNumber(repo.stargazers_count) }}</span><span>{{ relativeDate(repo.pushed_at) }}</span><span :class="['archive-state', { archived: repo.archived }]">{{ repo.archived ? t('archived') : t('active') }}</span><span v-if="repo.starred_at">{{ t('starredAt', { date: formatDate(repo.starred_at) }) }}</span></div></div></div><div class="repo-review"><div class="review-topline"><span class="decision-pill" :class="decisionClass(repo.decision)">{{ decisionLabel(repo.decision) }}</span><span class="score">{{ repo.score }}%</span></div><p class="review-summary">{{ repo.summary }}</p><div class="reason-list"><span v-for="reason in repo.reasons?.slice(0, 2)" :key="reason">{{ reason }}</span></div></div><div class="repo-actions"><a class="icon-button" :href="repo.html_url" target="_blank" rel="noreferrer" :title="t('openRepoTitle')">↗</a><button class="remove-button" :disabled="busyRepo === repo.full_name" @click="unstar(repo)">{{ busyRepo === repo.full_name ? '…' : (repo.archived ? t('findArchived') : t('unstarAction')) }}</button></div></article></div>
      </section>
    </main>
    <footer class="footer"><span>REVIEW/STARS</span><span>GitHub × AI × Telegram</span></footer>
  </div>
</template>
