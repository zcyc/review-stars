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
    noDescription: '这个仓库没有提供描述。', archived: '已归档', active: '未归档', fork: 'FORK', starredAt: '收藏于 {{date}}', openRepoTitle: '打开仓库', openStars: '打开 GitHub Stars', unstarAction: '取消 Star',
    decisionUnstar: '建议取消', decisionReview: '需要回顾', decisionKeep: '建议保留', pendingDecision: '待评审', confirmUnstar: '确定取消 {{repo}} 的 Star 吗？此操作会直接修改 GitHub。', unstarDone: '已取消 {{repo}} 的 Star',
    unknownDate: '未知', unknownUpdated: '更新时间未知', recentlyUpdated: '最近更新', todayActive: '今天有活动', daysAgo: '{{count}} 天前更新', monthsAgo: '{{count}} 个月前更新', yearsAgo: '{{count}} 年前更新', requestFailed: '请求失败（{{status}}）', switchToLight: '切换到亮色模式', switchToDark: '切换到暗色模式',
  },
  en: {
    connected: 'GitHub connected', waitingGithub: 'Waiting for GitHub configuration', reposLoaded: 'repositories loaded', noReposLoaded: 'No repositories loaded',
    sync: '↻ Sync repositories', syncing: 'Syncing…', startAI: 'Start AI review', rerunAI: 'Update AI review', viewingAI: 'AI is reviewing…', continueAI: 'Continue AI review (skip existing)', waitingAI: 'Waiting for AI…', aiCompleteButton: 'AI review complete', modelSuffix: '· OpenAI-compatible API',
    syncNotice: 'Synced {{count}} repositories from GitHub; existing reviews were reused', aiNotice: 'Finished {{total}} repositories: {{rules}} matched rules, reused {{cached}} reviews, and added {{reviewed}} AI reviews', continueNotice: 'AI review continued: {{rules}} matched rules, skipped {{cached}} existing reviews, and added {{reviewed}} new reviews',
    statCollected: 'Starred repositories', statCollectedFoot: 'Synced from GitHub', statUnstar: 'Suggested to unstar', statUnstarAI: 'AI and rule recommendations', statReview: 'Needs a human look', statReviewFoot: 'Worth opening once', statKeep: 'Suggested to keep', statKeepFoot: 'Still useful in your collection',
    randomTitle: 'Random repository recall', randomEmpty: 'Pick your next repositories to revisit', randomCount: 'Count', draw: 'Pick', repoListTitle: 'Repository list',
    searchPlaceholder: 'Search repositories…', all: 'All statuses', unstarFilter: 'Suggested to unstar', reviewFilter: 'Needs review', keepFilter: 'Suggested to keep', loadingStars: 'Reading local repository data…', noStarsTitle: 'No repositories synced', noStarsText: 'Click “Sync repositories” above to load all of your GitHub Stars.', noMatches: 'No matching repositories', noReview: 'No review results yet', noMatchesText: 'Try another search term or status filter.', startReviewHint: 'Use “Start AI review” above.',
    noDescription: 'This repository has no description.', archived: 'ARCHIVED', active: 'ACTIVE', fork: 'FORK', starredAt: 'Starred {{date}}', openRepoTitle: 'Open repository', openStars: 'Open GitHub Stars', unstarAction: 'Unstar', decisionUnstar: 'Suggested to unstar', decisionReview: 'Needs review', decisionKeep: 'Suggested to keep', pendingDecision: 'Pending review', confirmUnstar: 'Unstar {{repo}}? This will modify GitHub directly.', unstarDone: 'Unstarred {{repo}}',
    unknownDate: 'Unknown', unknownUpdated: 'Update date unknown', recentlyUpdated: 'Recently updated', todayActive: 'Active today', daysAgo: 'Updated {{count}} days ago', monthsAgo: 'Updated {{count}} months ago', yearsAgo: 'Updated {{count}} years ago', requestFailed: 'Request failed ({{status}})', switchToLight: 'Switch to light mode', switchToDark: 'Switch to dark mode',
  },
}

const state = {
  language: 'zh-CN',
  themeMode: 'system',
  systemDark: false,
  colorSchemeMedia: null,
  stars: [],
  reviews: [],
  stats: { total: 0, unstar: 0, review: 0, keep: 0 },
  health: null,
  aiComplete: false,
  randomRepos: [],
  randomCount: 1,
  randomizing: false,
  loadingStars: false,
  syncing: false,
  reviewing: false,
  initializing: true,
  busyRepo: '',
  error: '',
  errorLink: '',
  warning: '',
  notice: '',
  query: '',
  filter: 'all',
}

const app = document.querySelector('#app')

function t(key, variables = {}) {
  const dictionary = messages[state.language] || messages['zh-CN']
  const template = dictionary[key] || messages['zh-CN'][key] || key
  return Object.entries(variables).reduce((result, [name, value]) => result.replaceAll(`{{${name}}}`, String(value)), template)
}

function escapeHTML(value) {
  return String(value ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;')
}

function safeURL(value) {
  try {
    const url = new URL(value, window.location.origin)
    return url.protocol === 'http:' || url.protocol === 'https:' ? url.href : '#'
  } catch {
    return '#'
  }
}

function isDarkTheme() {
  return state.themeMode === 'dark' || (state.themeMode === 'system' && state.systemDark)
}

function applyTheme() {
  const root = document.documentElement
  root.classList.toggle('theme-light', state.themeMode === 'light')
  root.classList.toggle('theme-dark', state.themeMode === 'dark')
  root.style.colorScheme = state.themeMode === 'system' ? 'light dark' : state.themeMode
  const colorSchemeMeta = document.querySelector('meta[name="color-scheme"]')
  if (colorSchemeMeta) colorSchemeMeta.content = state.themeMode === 'system' ? 'light dark' : state.themeMode
  const themeColorMeta = document.querySelector('meta[name="theme-color"]')
  if (themeColorMeta) themeColorMeta.content = isDarkTheme() ? '#0d1117' : '#f6f8fa'
}

function initTheme() {
  const saved = window.localStorage.getItem('review-stars-theme')
  state.themeMode = saved === 'light' || saved === 'dark' ? saved : 'system'
  state.colorSchemeMedia = window.matchMedia('(prefers-color-scheme: dark)')
  state.systemDark = state.colorSchemeMedia.matches
  state.colorSchemeMedia.addEventListener('change', event => {
    state.systemDark = event.matches
    if (state.themeMode === 'system') applyTheme()
    renderHeader()
  })
  applyTheme()
}

function toggleTheme() {
  state.themeMode = isDarkTheme() ? 'light' : 'dark'
  window.localStorage.setItem('review-stars-theme', state.themeMode)
  applyTheme()
  renderHeader()
}

function renderShell() {
  app.innerHTML = `
    <div class="app-shell">
      <header class="topbar">
        <a class="brand" href="/">
          <span class="brand-star" aria-hidden="true"><svg viewBox="0 0 24 24" focusable="false"><path d="m12.672.668 3.059 6.197 6.838.993a.75.75 0 0 1 .416 1.28l-4.948 4.823 1.168 6.812a.75.75 0 0 1-1.088.79L12 18.347l-6.116 3.216a.75.75 0 0 1-1.088-.791l1.168-6.811-4.948-4.823a.749.749 0 0 1 .416-1.279l6.838-.994L11.327.668a.75.75 0 0 1 1.345 0Z"/></svg></span>
          <span class="brand-label">Review</span><span class="brand-ai">Stars</span>
        </a>
        <div class="topbar-end">
          <button class="theme-toggle" type="button" data-action="toggle-theme"></button>
          <div class="topbar-meta"><span id="live-dot" class="live-dot"></span><span id="github-status"></span></div>
        </div>
      </header>

      <main class="page-wrap">
        <h1 class="visually-hidden">Review Stars</h1>
        <section class="action-bar" aria-label="Actions">
          <div class="action-bar-meta"><strong id="repo-count"></strong><span id="model-note" class="model-note"></span></div>
          <div id="action-buttons" class="action-bar-actions"></div>
        </section>

        <div id="alerts" aria-live="polite"></div>
        <section id="stats-grid" class="stats-grid" aria-label="Repository statistics"></section>

        <section class="focus-grid">
          <article class="random-card">
            <div class="random-header"><h2 id="random-title"></h2><div class="random-actions"><label class="random-count"><span id="random-count-label"></span><input id="random-count" type="number" min="1" inputmode="numeric" /></label><button class="button button-dark" type="button" data-action="pick-random" id="random-button"></button></div></div>
            <div id="random-content"></div>
          </article>
        </section>

        <section class="review-section">
          <div class="section-heading"><h2 id="repo-list-title"></h2><search class="list-tools"><label class="search-box"><span aria-hidden="true">⌕</span><input id="repo-search" type="search" /></label><select id="repo-filter"></select></search></div>
          <div id="repository-content"></div>
        </section>
      </main>
      <footer class="footer"><span>REVIEW/STARS</span><span>GitHub × AI × Telegram</span></footer>
    </div>`
}

function renderHeader() {
  const themeButton = document.querySelector('[data-action="toggle-theme"]')
  if (themeButton) {
    const label = t(isDarkTheme() ? 'switchToLight' : 'switchToDark')
    themeButton.textContent = isDarkTheme() ? '☀' : '☾'
    themeButton.setAttribute('aria-label', label)
    themeButton.title = label
  }
  const dot = document.querySelector('#live-dot')
  if (dot) dot.classList.toggle('offline', Boolean(state.health && !state.health.github_configured))
  const status = document.querySelector('#github-status')
  if (status) status.textContent = state.health?.github_configured ? t('connected') : t('waitingGithub')
}

function renderActions() {
  const health = state.health || {}
  const count = document.querySelector('#repo-count')
  if (count) count.textContent = state.stars.length ? `${formatNumber(state.stars.length)} ${t('reposLoaded')}` : t('noReposLoaded')
  const model = document.querySelector('#model-note')
  if (model) model.textContent = `${health.ai_model || 'deepseek-v4-flash'} ${t('modelSuffix')}`
  const canSync = !state.initializing && !state.syncing && !state.reviewing && !state.busyRepo && health.github_configured
  const canReview = !state.initializing && !state.reviewing && !state.syncing && !state.busyRepo && health.ai_configured && state.stars.length > 0
  const canContinue = canReview && !state.aiComplete
  document.querySelector('#action-buttons').innerHTML = `
    <button class="button button-outline" type="button" data-action="sync" ${canSync ? '' : 'disabled'}>${state.syncing ? '<span class="spinner"></span>' : ''}${state.syncing ? t('syncing') : t('sync')}</button>
    <button class="button button-primary" type="button" data-action="review" ${canReview ? '' : 'disabled'}>${state.reviewing ? '<span class="spinner"></span>' : ''}${state.reviewing ? t('viewingAI') : (state.reviews.length ? t('rerunAI') : t('startAI'))}</button>
    <button class="button button-outline" type="button" data-action="continue-review" ${canContinue ? '' : 'disabled'}>${state.aiComplete ? t('aiCompleteButton') : (state.reviewing ? t('waitingAI') : t('continueAI'))}</button>`
}

function renderAlerts() {
  const alerts = document.querySelector('#alerts')
  if (!alerts) return
  alerts.innerHTML = [
    state.error && `<div class="alert alert-error"><span aria-hidden="true">!</span><span>${escapeHTML(state.error)}</span>${state.errorLink ? `<a href="${escapeHTML(safeURL(state.errorLink))}" target="_blank" rel="noreferrer noopener">${t('openStars')}</a>` : ''}</div>`,
    state.warning && `<div class="alert alert-warning"><span aria-hidden="true">△</span><span>${escapeHTML(state.warning)}</span></div>`,
    state.notice && `<div class="alert alert-success"><span aria-hidden="true">✓</span><span>${escapeHTML(state.notice)}</span></div>`,
  ].filter(Boolean).join('')
}

function renderStats() {
  const stats = state.stats
  document.querySelector('#stats-grid').innerHTML = `
    <article class="stat-card stat-total"><div class="stat-label">${t('statCollected')}</div><div class="stat-value">${formatNumber(stats.total || state.stars.length)}</div><div class="stat-foot">${t('statCollectedFoot')}</div></article>
    <article class="stat-card stat-danger"><div class="stat-label">${t('statUnstar')}</div><div class="stat-value">${formatNumber(stats.unstar)}</div><div class="stat-foot">${t('statUnstarAI')}</div></article>
    <article class="stat-card stat-warm"><div class="stat-label">${t('statReview')}</div><div class="stat-value">${formatNumber(stats.review)}</div><div class="stat-foot">${t('statReviewFoot')}</div></article>
    <article class="stat-card stat-green"><div class="stat-label">${t('statKeep')}</div><div class="stat-value">${formatNumber(stats.keep)}</div><div class="stat-foot">${t('statKeepFoot')}</div></article>`
}

function renderRandom() {
  document.querySelector('#random-title').textContent = t('randomTitle')
  document.querySelector('#random-count-label').textContent = t('randomCount')
  document.querySelector('#random-count').value = state.randomCount
  const randomButton = document.querySelector('#random-button')
  randomButton.textContent = t('draw')
  randomButton.disabled = state.randomizing
  const content = document.querySelector('#random-content')
  if (!state.randomRepos.length) {
    content.innerHTML = `<div class="random-placeholder"><span aria-hidden="true">✦</span><span>${t('randomEmpty')}</span></div>`
    return
  }
  content.innerHTML = `<div class="random-results">${state.randomRepos.map(repo => `
    <div class="random-result"><div class="repo-avatar">${escapeHTML(repo.name?.slice(0, 1).toUpperCase())}</div>
      <div class="random-info"><a href="${escapeHTML(safeURL(repo.html_url))}" target="_blank" rel="noreferrer noopener">${escapeHTML(repo.full_name)}</a><span>${escapeHTML(repo.summary || repo.description || t('noDescription'))}</span></div>
      <span class="decision-pill ${decisionClass(repo.decision)}">${decisionLabel(repo.decision)}</span>
    </div>`).join('')}</div>`
}

function renderRepositoryControls() {
  document.querySelector('#repo-list-title').textContent = t('repoListTitle')
  const search = document.querySelector('#repo-search')
  search.placeholder = t('searchPlaceholder')
  search.setAttribute('aria-label', t('searchPlaceholder'))
  search.value = state.query
  const filter = document.querySelector('#repo-filter')
  filter.innerHTML = `<option value="all">${t('all')}</option><option value="unstar">${t('unstarFilter')}</option><option value="review">${t('reviewFilter')}</option><option value="keep">${t('keepFilter')}</option>`
  filter.value = state.filter
  filter.setAttribute('aria-label', t('all'))
}

function visibleReviews() {
  const reviewByName = new Map(state.reviews.map(review => [repoKey(review.full_name), review]))
  const source = state.stars.length
    ? state.stars.map(repo => reviewByName.get(repoKey(repo.full_name)) || {
        ...repo,
        decision: 'review',
        score: 0,
        summary: t('noReview'),
        reasons: [t('startReviewHint')],
      })
    : state.reviews
  const needle = state.query.trim().toLowerCase()
  return source.filter(repo => {
    const matchesQuery = !needle || [repo.full_name, repo.description, repo.language].filter(Boolean).some(value => value.toLowerCase().includes(needle))
    const matchesFilter = state.filter === 'all' || repo.decision === state.filter
    return matchesQuery && matchesFilter
  })
}

function repoKey(fullName) {
  return String(fullName || '').trim().toLowerCase()
}

function renderRepositoryContent() {
  const content = document.querySelector('#repository-content')
  if (state.loadingStars) {
    content.innerHTML = `<div class="empty-state"><span class="spinner spinner-dark"></span><p>${t('loadingStars')}</p></div>`
    return
  }
  if (!state.stars.length) {
    content.innerHTML = `<div class="empty-state"><div class="empty-icon">☆</div><h3>${t('noStarsTitle')}</h3><p>${t('noStarsText')}</p></div>`
    return
  }
  const reviews = visibleReviews()
  if (!reviews.length) {
    content.innerHTML = `<div class="empty-state"><div class="empty-icon">⌕</div><h3>${state.reviews.length ? t('noMatches') : t('noReview')}</h3><p>${state.reviews.length ? t('noMatchesText') : t('startReviewHint')}</p></div>`
    return
  }
  content.innerHTML = `<div class="repo-list">${reviews.map(renderRepository).join('')}</div>`
}

function renderRepository(repo) {
  const archived = repo.archived ? `<span class="tiny-tag">${t('archived')}</span>` : ''
  const fork = repo.fork ? `<span class="tiny-tag">${t('fork')}</span>` : ''
  const language = repo.language ? `<span><i class="language-dot"></i>${escapeHTML(repo.language)}</span>` : ''
  const starredAt = repo.starred_at ? `<span>${t('starredAt', { date: escapeHTML(formatDate(repo.starred_at)) })}</span>` : ''
  const reasons = (repo.reasons || []).slice(0, 2).map(reason => `<span>${escapeHTML(reason)}</span>`).join('')
  const actionLabel = state.busyRepo === repo.full_name ? '…' : t('unstarAction')
  return `<article class="repo-row">
    <div class="repo-main"><div class="repo-avatar repo-avatar-large">${escapeHTML(repo.name?.slice(0, 1).toUpperCase())}</div><div class="repo-copy">
      <div class="repo-title-line"><a href="${escapeHTML(safeURL(repo.html_url))}" target="_blank" rel="noreferrer noopener">${escapeHTML(repo.full_name)}</a>${archived}${fork}</div>
      <p>${escapeHTML(repo.description || t('noDescription'))}</p>
      <div class="repo-meta">${language}<span>★ ${formatNumber(repo.stargazers_count)}</span><span>${relativeDate(repo.pushed_at)}</span><span class="archive-state${repo.archived ? ' archived' : ''}">${repo.archived ? t('archived') : t('active')}</span>${starredAt}</div>
    </div></div>
    <div class="repo-review"><div class="review-topline"><span class="decision-pill ${decisionClass(repo.decision)}">${decisionLabel(repo.decision)}</span><span class="score">${escapeHTML(repo.score)}%</span></div><p class="review-summary">${escapeHTML(repo.summary)}</p><div class="reason-list">${reasons}</div></div>
    <div class="repo-actions"><a class="icon-button" href="${escapeHTML(safeURL(repo.html_url))}" target="_blank" rel="noreferrer noopener" title="${t('openRepoTitle')}">↗</a><button class="remove-button" type="button" data-action="unstar" data-repo="${escapeHTML(repo.full_name)}" ${state.busyRepo === repo.full_name ? 'disabled' : ''}>${actionLabel}</button></div>
  </article>`
}

function render() {
  renderHeader()
  renderActions()
  renderAlerts()
  renderStats()
  renderRandom()
  renderRepositoryControls()
  renderRepositoryContent()
}

function decisionLabel(decision) {
  return { unstar: t('decisionUnstar'), review: t('decisionReview'), keep: t('decisionKeep') }[decision] || t('pendingDecision')
}

function decisionClass(decision) {
  return `decision-${['unstar', 'review', 'keep'].includes(decision) ? decision : 'review'}`
}

function formatNumber(value) {
  return new Intl.NumberFormat(state.language === 'en' ? 'en-US' : 'zh-CN').format(value || 0)
}

function formatDate(date) {
  if (!date) return t('unknownDate')
  const value = new Date(date)
  if (Number.isNaN(value.getTime())) return t('unknownDate')
  return new Intl.DateTimeFormat(state.language === 'en' ? 'en-US' : 'zh-CN', { year: 'numeric', month: 'short', day: 'numeric' }).format(value)
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
    state.health = data
    state.language = data.language || 'zh-CN'
    document.documentElement.lang = state.language === 'en' ? 'en' : 'zh-CN'
    render()
  } catch (error) {
    state.errorLink = ''
    state.error = error.message
    render()
  }
}

async function loadStars() {
  state.loadingStars = true
  renderRepositoryContent()
  try {
    const data = await request('/api/stars')
    state.stars = data.repositories || []
  } catch (error) {
    state.errorLink = ''
    state.error = error.message
  } finally {
    state.loadingStars = false
    render()
  }
}

async function syncStars() {
  state.syncing = true
  state.error = ''
  state.errorLink = ''
  state.warning = ''
  state.notice = ''
  render()
  try {
    const data = await request('/api/sync', { method: 'POST' })
    state.stars = data.repositories || []
    state.reviews = data.reviews || []
    state.stats = data.stats || state.stats
    state.aiComplete = Boolean(data.ai_complete)
    state.notice = t('syncNotice', { count: state.stars.length })
  } catch (error) {
    state.errorLink = ''
    state.error = error.message
  } finally {
    state.syncing = false
    render()
  }
}

async function loadExistingReview() {
  try {
    const data = await request('/api/review')
    state.reviews = data.reviews || []
    state.stats = data.stats || state.stats
    state.aiComplete = Boolean(data.ai_complete)
    render()
  } catch (error) {
    state.errorLink = ''
    state.error = error.message
    render()
  }
}

async function runAIReview(continuing = false) {
  state.reviewing = true
  state.error = ''
  state.errorLink = ''
  state.warning = ''
  state.notice = ''
  render()
  try {
    const queryString = continuing ? '?continue=1' : ''
    const data = await request(`/api/review${queryString}`, { method: 'POST' })
    state.reviews = data.reviews || []
    state.stats = data.stats || state.stats
    state.aiComplete = Boolean(data.ai_complete)
    state.warning = data.warning || ''
    state.notice = continuing
      ? t('continueNotice', { rules: data.rule_matched_count || 0, cached: data.cached_count || 0, reviewed: data.ai_reviewed_count || 0 })
      : t('aiNotice', { total: state.stats.total, rules: data.rule_matched_count || 0, cached: data.cached_count || 0, reviewed: data.ai_reviewed_count || 0 })
  } catch (error) {
    state.errorLink = ''
    state.error = error.message
  } finally {
    state.reviewing = false
    render()
  }
}

async function pickRandom() {
  if (state.randomizing) return
  state.randomizing = true
  state.error = ''
  state.errorLink = ''
  const count = Math.max(1, Number(state.randomCount) || 1)
  state.randomCount = count
  renderRandom()
  try {
    const data = await request(`/api/random?count=${encodeURIComponent(count)}`)
    state.randomRepos = data.repositories || []
  } catch (error) {
    state.errorLink = ''
    state.error = error.message
  }
  state.randomizing = false
  render()
}

async function unstar(repo) {
  state.error = ''
  state.errorLink = ''
  state.notice = ''
  if (!window.confirm(t('confirmUnstar', { repo: repo.full_name }))) return
  state.busyRepo = repo.full_name
  renderRepositoryContent()
  try {
    await request(`/api/stars/${encodeURIComponent(repo.full_name)}`, { method: 'DELETE' })
    const key = repoKey(repo.full_name)
    state.stars = state.stars.filter(item => repoKey(item.full_name) !== key)
    const aiRepo = state.reviews.find(item => repoKey(item.full_name) === key)
    state.reviews = state.reviews.filter(item => repoKey(item.full_name) !== key)
    state.randomRepos = state.randomRepos.filter(item => repoKey(item.full_name) !== key)
    const decision = aiRepo?.decision === 'unstar' ? 'unstar' : aiRepo?.decision === 'keep' ? 'keep' : 'review'
    state.stats = {
      ...state.stats,
      total: Math.max(0, state.stats.total - 1),
      [decision]: Math.max(0, state.stats[decision] - 1),
    }
    state.notice = t('unstarDone', { repo: repo.full_name })
  } catch (error) {
    state.error = error.message
    if (error.status === 403 && error.body?.stars_url) state.errorLink = error.body.stars_url
  } finally {
    state.busyRepo = ''
    render()
  }
}

app.addEventListener('click', event => {
  const action = event.target.closest('[data-action]')?.dataset.action
  if (!action) return
  if (action === 'toggle-theme') toggleTheme()
  if (action === 'sync') syncStars()
  if (action === 'review') runAIReview()
  if (action === 'continue-review') runAIReview(true)
  if (action === 'pick-random') pickRandom()
  if (action === 'unstar') {
    const repo = state.stars.find(item => item.full_name === event.target.closest('[data-action="unstar"]').dataset.repo)
    if (repo) unstar(repo)
  }
})

app.addEventListener('input', event => {
  if (event.target.id === 'repo-search') {
    state.query = event.target.value
    renderRepositoryContent()
  }
  if (event.target.id === 'random-count') state.randomCount = event.target.value
})

app.addEventListener('change', event => {
  if (event.target.id === 'repo-filter') {
    state.filter = event.target.value
    renderRepositoryContent()
  }
})

renderShell()
initTheme()
render()
Promise.all([loadHealth(), loadStars(), loadExistingReview()]).finally(() => {
  state.initializing = false
  render()
})
