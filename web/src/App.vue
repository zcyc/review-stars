<script setup>
import { computed, onMounted, ref } from 'vue'

const stars = ref([])
const reviews = ref([])
const ruleReviews = ref([])
const stats = ref({ total: 0, unstar: 0, review: 0, keep: 0 })
const ruleStats = ref({ total: 0, unstar: 0, review: 0, keep: 0 })
const health = ref(null)
const randomRepo = ref(null)
const loadingStars = ref(false)
const syncing = ref(false)
const reviewing = ref(false)
const ruleReviewing = ref(false)
const reminding = ref(false)
const busyRepo = ref('')
const error = ref('')
const warning = ref('')
const notice = ref('')
const query = ref('')
const filter = ref('all')
const reviewMode = ref('ai')

const activeReviews = computed(() => reviewMode.value === 'rule' ? ruleReviews.value : reviews.value)
const activeStats = computed(() => reviewMode.value === 'rule' ? ruleStats.value : stats.value)

const visibleReviews = computed(() => {
  const reviewByName = new Map(activeReviews.value.map(review => [review.full_name, review]))
  const source = stars.value.length
    ? stars.value.map(repo => reviewByName.get(repo.full_name) || {
        ...repo,
        decision: 'review',
        score: 0,
        summary: reviewMode.value === 'rule' ? '还没有规则评审结果' : '还没有 AI 评审结果',
        reasons: [reviewMode.value === 'rule' ? '点击“运行规则评审”获取建议' : '点击“开始 AI 评审”获取建议'],
        next_action: '开始评审',
        source: 'pending',
      })
    : activeReviews.value
  const needle = query.value.trim().toLowerCase()
  return source.filter(repo => {
    const matchesQuery = !needle || [repo.full_name, repo.description, repo.language]
      .filter(Boolean)
      .some(value => value.toLowerCase().includes(needle))
    const matchesFilter = filter.value === 'all' || repo.decision === filter.value
    return matchesQuery && matchesFilter
  })
})

const hasReviews = computed(() => reviews.value.length > 0)

async function request(path, options = {}) {
  const response = await fetch(path, {
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options,
  })
  const body = await response.json().catch(() => ({}))
  if (!response.ok) throw new Error(body.error || `请求失败（${response.status}）`)
  return body
}

async function loadHealth() {
  try {
    health.value = await request('/api/health')
  } catch (err) {
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
    error.value = err.message
  } finally {
    loadingStars.value = false
  }
}

async function syncStars() {
  syncing.value = true
  error.value = ''
  warning.value = ''
  notice.value = ''
  try {
    const data = await request('/api/sync', { method: 'POST' })
    stars.value = data.repositories || []
    reviews.value = data.reviews || []
    ruleReviews.value = data.rule_reviews || []
    stats.value = data.stats || stats.value
    ruleStats.value = data.rule_stats || ruleStats.value
    notice.value = `已从 GitHub 同步 ${stars.value.length} 个仓库，已有 review 已复用`
  } catch (err) {
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
  } catch {
    // A first visit has no review yet; the empty state is intentional.
  }
}

async function loadExistingRuleReview() {
  try {
    const data = await request('/api/rule-review')
    ruleReviews.value = data.reviews || []
    ruleStats.value = data.stats || ruleStats.value
  } catch {
    // A first visit has no rule review yet; the empty state is intentional.
  }
}

async function runReview() {
  reviewing.value = true
  error.value = ''
  warning.value = ''
  notice.value = ''
  try {
    const data = await request(`/api/review${hasReviews.value ? '?force=1' : ''}`, { method: 'POST' })
    reviewMode.value = 'ai'
    reviews.value = data.reviews || []
    stats.value = data.stats || stats.value
    warning.value = data.warning || ''
    notice.value = `已完成 ${stats.value.total} 个仓库：复用已有 review ${data.cached_count || 0} 个，AI 本次评审 ${data.ai_reviewed_count || 0} 个`
  } catch (err) {
    error.value = err.message
  } finally {
    reviewing.value = false
  }
}

async function runRuleReview() {
  ruleReviewing.value = true
  error.value = ''
  warning.value = ''
  notice.value = ''
  try {
    const data = await request('/api/rule-review', { method: 'POST' })
    ruleReviews.value = data.reviews || []
    ruleStats.value = data.stats || ruleStats.value
    reviewMode.value = 'rule'
    filter.value = 'unstar'
    notice.value = `规则评审完成：${ruleStats.value.unstar} 个仓库命中规则`
  } catch (err) {
    error.value = err.message
  } finally {
    ruleReviewing.value = false
  }
}

async function pickRandom() {
  error.value = ''
  try {
    const data = await request('/api/random')
    randomRepo.value = data.repository
  } catch (err) {
    error.value = err.message
  }
}

async function sendReminder() {
  reminding.value = true
  error.value = ''
  try {
    const data = await request('/api/remind', { method: 'POST' })
    randomRepo.value = data.repository
    notice.value = '已通过 Telegram 发出回顾提醒'
  } catch (err) {
    error.value = err.message
  } finally {
    reminding.value = false
  }
}

async function unstar(repo) {
  if (!window.confirm(`确定取消 ${repo.full_name} 的 Star 吗？此操作会直接修改 GitHub。`)) return
  busyRepo.value = repo.full_name
  error.value = ''
  try {
    await request(`/api/stars/${encodeURIComponent(repo.full_name)}`, { method: 'DELETE' })
    stars.value = stars.value.filter(item => item.full_name !== repo.full_name)
    const aiRepo = reviews.value.find(item => item.full_name === repo.full_name)
    const ruleRepo = ruleReviews.value.find(item => item.full_name === repo.full_name)
    reviews.value = reviews.value.filter(item => item.full_name !== repo.full_name)
    ruleReviews.value = ruleReviews.value.filter(item => item.full_name !== repo.full_name)
    const decrement = (current, removed) => removed ? { ...current, total: Math.max(0, current.total - 1), unstar: Math.max(0, current.unstar - (removed.decision === 'unstar' ? 1 : 0)) } : current
    stats.value = decrement(stats.value, aiRepo)
    ruleStats.value = decrement(ruleStats.value, ruleRepo)
    notice.value = `已取消 ${repo.full_name} 的 Star`
  } catch (err) {
    error.value = err.message
  } finally {
    busyRepo.value = ''
  }
}

function decisionLabel(decision) {
  return { unstar: '建议取消', review: '需要回顾', keep: '建议保留' }[decision] || '待评审'
}

function decisionClass(decision) {
  return `decision-${decision || 'review'}`
}

function formatDate(date) {
  if (!date) return '未知'
  const value = new Date(date)
  if (Number.isNaN(value.getTime())) return '未知'
  return new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: 'short', day: 'numeric' }).format(value)
}

function relativeDate(date) {
  if (!date) return '未知'
  const value = new Date(date)
  if (Number.isNaN(value.getTime()) || value.getUTCFullYear() < 2000) return '更新时间未知'
  const days = Math.floor((Date.now() - value.getTime()) / 86400000)
  if (days < 0) return '最近更新'
  if (days < 1) return '今天有活动'
  if (days < 30) return `${days} 天前更新`
  if (days < 365) return `${Math.floor(days / 30)} 个月前更新`
  return `${Math.floor(days / 365)} 年前更新`
}

onMounted(async () => {
  await Promise.all([loadHealth(), loadStars(), loadExistingReview(), loadExistingRuleReview()])
})
</script>

<template>
  <div class="app-shell">
    <header class="topbar">
      <a class="brand" href="/">
        <span class="brand-mark">✦</span>
        <span>REVIEW<span class="brand-muted">/</span>STARS</span>
      </a>
      <div class="topbar-meta">
        <span class="live-dot" :class="{ offline: health && !health.github_configured }"></span>
        {{ health?.github_configured ? 'GitHub 已连接' : '等待 GitHub 配置' }}
      </div>
    </header>

    <main class="page-wrap">
      <section class="hero">
        <div>
          <p class="eyebrow">PERSONAL REPOSITORY HYGIENE</p>
          <h1>让每一个 Star<br /><em>保持有用。</em></h1>
          <p class="hero-copy">把收藏夹里的仓库重新变成你的工具箱。AI 负责理解上下文，规则评审负责按状态、更新时间和 Star 数快速筛选。</p>
        </div>
        <div class="hero-actions">
          <button class="button button-outline" :disabled="syncing || !health?.github_configured" @click="syncStars">
            <span v-if="syncing" class="spinner"></span>
            <span>{{ syncing ? '同步中…' : '↻ 同步仓库' }}</span>
          </button>
          <button class="button button-primary" :disabled="reviewing || !health?.github_configured || !health?.openrouter_configured || !stars.length" @click="runReview">
            <span v-if="reviewing" class="spinner"></span>
            <span>{{ reviewing ? 'AI 正在查看…' : (hasReviews ? '重新 AI 评审' : '开始 AI 评审') }}</span>
          </button>
          <button class="button button-outline" :disabled="ruleReviewing || !stars.length" @click="runRuleReview">
            <span v-if="ruleReviewing" class="spinner"></span>
            <span>{{ ruleReviewing ? '规则计算中…' : '运行规则评审' }}</span>
          </button>
          <span class="model-note">{{ health?.model || 'openrouter/free' }} · 免费模型路由</span>
        </div>
      </section>

      <div v-if="error" class="alert alert-error"><span>!</span>{{ error }}</div>
      <div v-if="warning" class="alert alert-warning"><span>△</span>{{ warning }}</div>
      <div v-if="notice" class="alert alert-success"><span>✓</span>{{ notice }}</div>

      <section class="stats-grid">
        <article class="stat-card stat-total">
          <div class="stat-label">已收藏仓库</div>
          <div class="stat-value">{{ activeStats.total || stars.length }}</div>
          <div class="stat-foot">同步自 GitHub</div>
        </article>
        <article class="stat-card stat-danger">
          <div class="stat-label">建议取消 Star</div>
          <div class="stat-value">{{ activeStats.unstar }}</div>
          <div class="stat-foot">{{ reviewMode === 'rule' ? '命中配置规则' : 'AI 建议取消' }}</div>
        </article>
        <article class="stat-card stat-warm">
          <div class="stat-label">需要人工回顾</div>
          <div class="stat-value">{{ activeStats.review }}</div>
          <div class="stat-foot">值得打开看一眼</div>
        </article>
        <article class="stat-card stat-green">
          <div class="stat-label">建议保留</div>
          <div class="stat-value">{{ activeStats.keep }}</div>
          <div class="stat-foot">仍然值得放在收藏夹</div>
        </article>
      </section>

      <section class="focus-grid">
        <article class="random-card">
          <div class="section-kicker">RANDOM RECALL <span>01</span></div>
          <h2>随机回顾一个仓库</h2>
          <p>不必一次清理全部收藏。每天打开一个，给它一个决定。</p>
          <div v-if="randomRepo" class="random-result">
            <div class="repo-avatar">{{ randomRepo.name?.slice(0, 1).toUpperCase() }}</div>
            <div class="random-info">
              <a :href="randomRepo.html_url" target="_blank" rel="noreferrer">{{ randomRepo.full_name }}</a>
              <span>{{ randomRepo.summary || randomRepo.description || '打开仓库，重新认识它。' }}</span>
            </div>
            <span class="decision-pill" :class="decisionClass(randomRepo.decision)">{{ decisionLabel(randomRepo.decision) }}</span>
          </div>
          <div v-else class="random-placeholder"><span>✦</span><span>点击按钮抽取你的下一个回顾对象</span></div>
          <div class="random-actions">
            <button class="button button-dark" @click="pickRandom">抽一个</button>
            <button class="button button-outline" :disabled="reminding" @click="sendReminder">
              {{ reminding ? '发送中…' : '✈ Telegram 提醒' }}
            </button>
          </div>
        </article>

        <article class="insight-card">
          <div class="section-kicker">HOW IT WORKS <span>02</span></div>
          <h2>把清理变成轻量习惯</h2>
          <div class="how-row"><span class="how-number">01</span><div><strong>AI 先看信号</strong><p>归档、活跃度、项目类型与上下文一起判断。</p></div></div>
          <div class="how-row"><span class="how-number">02</span><div><strong>你来做最后决定</strong><p>建议永远不会自动取消 Star，操作权在你手上。</p></div></div>
          <div class="how-row"><span class="how-number">03</span><div><strong>Telegram 负责提醒</strong><p>随机挑一个仓库，在你方便时把它带回来。</p></div></div>
        </article>
      </section>

      <section class="review-section">
        <div class="section-heading">
          <div>
            <div class="section-kicker">YOUR COLLECTION <span>03</span></div>
            <h2>{{ reviewMode === 'rule' ? '规则命中清单' : '仓库清单' }}</h2>
          </div>
          <div class="list-tools">
            <div class="review-modes">
              <button :class="['mode-button', { active: reviewMode === 'ai' }]" @click="reviewMode = 'ai'">AI review</button>
              <button :class="['mode-button', { active: reviewMode === 'rule' }]" @click="reviewMode = 'rule'">规则 review</button>
            </div>
            <label class="search-box"><span>⌕</span><input v-model="query" placeholder="搜索仓库…" /></label>
            <select v-model="filter"><option value="all">全部状态</option><option value="unstar">建议取消</option><option value="review">需要回顾</option><option value="keep">建议保留</option></select>
          </div>
        </div>

        <div v-if="loadingStars" class="empty-state"><span class="spinner spinner-dark"></span><p>正在读取本地仓库数据…</p></div>
        <div v-else-if="!stars.length" class="empty-state"><div class="empty-icon">☆</div><h3>还没有同步仓库</h3><p>点击上方“同步仓库”从 GitHub 获取全部 Stars。</p></div>
        <div v-else-if="!visibleReviews.length" class="empty-state"><div class="empty-icon">⌕</div><h3>{{ activeReviews.length ? '没有匹配的仓库' : (reviewMode === 'rule' ? '还没有规则评审结果' : '还没有 AI 评审结果') }}</h3><p>{{ activeReviews.length ? '试试换一个搜索词或状态筛选。' : '点击上方对应按钮开始评审。' }}</p></div>
        <div v-else class="repo-list">
          <article v-for="repo in visibleReviews" :key="repo.full_name" class="repo-row">
            <div class="repo-main">
              <div class="repo-avatar repo-avatar-large">{{ repo.name?.slice(0, 1).toUpperCase() }}</div>
              <div class="repo-copy">
                <div class="repo-title-line"><a :href="repo.html_url" target="_blank" rel="noreferrer">{{ repo.full_name }}</a><span v-if="repo.archived" class="tiny-tag">ARCHIVED</span><span v-if="repo.fork" class="tiny-tag">FORK</span></div>
                <p>{{ repo.description || '这个仓库没有提供描述。' }}</p>
                <div class="repo-meta"><span v-if="repo.language"><i class="language-dot"></i>{{ repo.language }}</span><span>★ {{ repo.stargazers_count?.toLocaleString() || 0 }}</span><span>{{ relativeDate(repo.pushed_at) }}</span><span :class="['archive-state', { archived: repo.archived }]">{{ repo.archived ? '已归档' : '未归档' }}</span><span v-if="repo.starred_at">收藏于 {{ formatDate(repo.starred_at) }}</span></div>
              </div>
            </div>
            <div class="repo-review">
              <div class="review-topline"><span class="decision-pill" :class="decisionClass(repo.decision)">{{ decisionLabel(repo.decision) }}</span><span class="source-tag">{{ repo.source === 'rule' ? 'RULE' : (repo.source === 'openrouter' ? 'AI' : 'PENDING') }}</span><span class="score">{{ repo.score }}%</span></div>
              <p class="review-summary">{{ repo.summary }}</p>
              <div class="reason-list"><span v-for="reason in repo.reasons?.slice(0, 2)" :key="reason">{{ reason }}</span></div>
            </div>
            <div class="repo-actions"><a class="icon-button" :href="repo.html_url" target="_blank" rel="noreferrer" title="打开仓库">↗</a><button v-if="repo.decision === 'unstar'" class="remove-button" :disabled="busyRepo === repo.full_name" @click="unstar(repo)">{{ busyRepo === repo.full_name ? '…' : '取消 Star' }}</button></div>
          </article>
        </div>
      </section>
    </main>

    <footer class="footer"><span>REVIEW/STARS</span><span>GitHub × OpenRouter × Telegram</span></footer>
  </div>
</template>
