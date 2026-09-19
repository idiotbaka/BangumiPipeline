<script setup lang="ts">
import { computed, ref } from 'vue'
import report from '../seasonal/202610/data.json'

interface PersonGroup { role: string; names: string[] }
interface ReportWork {
  id: string
  title: string
  originalTitle: string
  tags: string[]
  schedule: string
  premiereAt: string
  story: string
  cast: PersonGroup[]
  staff: PersonGroup[]
  sourceUrl: string
  cover: string
}

type SortMode = 'premiere' | 'title' | 'category'
const castPreviewLimit = 10
const staffPreviewLimit = 10
const works = report.works as ReportWork[]
const query = ref('')
const category = ref('all')
const origin = ref('all')
const genre = ref('all')
const sortMode = ref<SortMode>('premiere')
const feature = works.slice(0, 3)

const scheduledCount = works.filter((work) => work.premiereAt).length
const newCount = works.filter((work) => work.tags.includes('新作')).length
const origins = computed(() => [...new Set(works.flatMap((work) => work.tags.filter((tag) => /改編|原創/.test(tag))))].sort())
const genreOptions = computed(() => {
  const excluded = new Set(['新作', '續作', ...origins.value])
  const count = new Map<string, number>()
  works.forEach((work) => work.tags.forEach((tag) => {
    if (!excluded.has(tag) && tag !== '家長指引' && !/^\d+(?:集|季度|季)$/.test(tag)) {
      count.set(tag, (count.get(tag) ?? 0) + 1)
    }
  }))
  return [...count].filter(([, total]) => total >= 2).sort((a, b) => b[1] - a[1]).map(([name]) => name)
})

const visibleWorks = computed(() => {
  const search = query.value.trim().toLocaleLowerCase()
  const results = works.filter((work) => {
    if (category.value !== 'all' && !work.tags.includes(category.value)) return false
    if (origin.value !== 'all' && !work.tags.includes(origin.value)) return false
    if (genre.value !== 'all' && !work.tags.includes(genre.value)) return false
    if (search && !`${work.title} ${work.originalTitle}`.toLocaleLowerCase().includes(search)) return false
    return true
  })
  results.sort((a, b) => {
    if (sortMode.value === 'title') return a.title.localeCompare(b.title, 'zh-Hans-CN')
    if (sortMode.value === 'category') return (a.tags.includes('續作') ? 1 : 0) - (b.tags.includes('續作') ? 1 : 0)
      || comparePremiere(a, b)
    return comparePremiere(a, b)
  })
  return results
})

function comparePremiere(a: ReportWork, b: ReportWork) {
  if (!a.premiereAt) return b.premiereAt ? 1 : a.title.localeCompare(b.title)
  if (!b.premiereAt) return -1
  return a.premiereAt.localeCompare(b.premiereAt) || a.title.localeCompare(b.title)
}

function dateBadge(work: ReportWork) {
  if (!work.premiereAt) return '待定'
  const match = work.schedule.match(/(\d{1,2})月(\d{1,2})日/)
  return match ? `${Number(match[1])}月${Number(match[2])}日` : work.premiereAt.slice(5).replace('-', '月') + '日'
}

function categoryLabel(work: ReportWork) {
  return work.tags.includes('續作') ? '续作' : '新作'
}

function featuredStaff(work: ReportWork) {
  if (work.staff.length <= staffPreviewLimit) return work.staff
  const first = work.staff.slice(0, staffPreviewLimit)
  const studio = work.staff.find((person) => /^(?:動畫製作|动画制作|アニメーション制作)$/.test(person.role))
  if (studio && !first.includes(studio)) first[staffPreviewLimit - 1] = studio
  return first
}

function resetFilters() {
  query.value = ''
  category.value = 'all'
  origin.value = 'all'
  genre.value = 'all'
  sortMode.value = 'premiere'
}

</script>

<template>
  <section class="season-preview-stage" aria-label="10月秋番报告">
    <div class="preview-grid" aria-hidden="true" />
    <div class="preview-halo preview-halo-pink" aria-hidden="true" />
    <div class="preview-halo preview-halo-cyan" aria-hidden="true" />
    <div class="preview-wrap">
      <div class="preview-hero">
        <div class="hero-copy">
          <div class="hero-kicker"><span class="kicker-dot" /> NEXT SEASON REPORT <span class="kicker-line" /> 2026.10</div>
          <p class="hero-overline">2026 秋季 · 新番前瞻</p>
          <h1>下一站，<br /><em>十月。</em></h1>
          <p class="hero-description">从第一声开播铃响起，到值得期待的每一部新作。<br />把秋天的故事，提前翻开一页。</p>
          <div class="hero-actions">
            <a class="primary-action" href="#october-lineup">探索本季阵容 <span aria-hidden="true">↗</span></a>
            <span>JAPAN TV ANIME · AUTUMN 2026</span>
          </div>
        </div>
        <div class="hero-art" aria-hidden="true">
          <div class="art-orbit orbit-one" /><div class="art-orbit orbit-two" />
          <div class="art-stamp">OCT<br /><strong>26</strong></div>
          <div v-for="(work, index) in feature" :key="work.id" class="art-cover" :class="`cover-${index + 1}`">
            <img :src="work.cover" alt="" />
          </div>
          <span class="art-caption">A NEW CHAPTER<br />IS ABOUT TO BEGIN</span>
        </div>
      </div>

      <div class="season-facts" aria-label="本季概览">
        <div><strong>{{ works.length }}</strong><span>部收录作品</span><small>THE LINEUP</small></div>
        <div><strong>{{ newCount }}</strong><span>部全新作品</span><small>NEW STORIES</small></div>
        <div><strong>{{ scheduledCount }}</strong><span>部已公布档期</span><small>ON THE CALENDAR</small></div>
        <p>资料快照<br /><b>{{ report.snapshotDate }}</b><br /><small>时间均为日本时间</small></p>
      </div>

      <div id="october-lineup" class="lineup-heading">
        <div>
          <p class="section-kicker"><span>01</span> / THE COMPLETE LINEUP</p>
          <h2>本季新番图鉴<span>✦</span></h2>
          <p>从首播档期到声优与制作阵容，收藏你的秋日待看清单。</p>
        </div>
        <div class="lineup-mark">AUTUMN<br /><strong>2026</strong></div>
      </div>

      <div class="filter-panel">
        <div class="filter-top">
          <div class="category-tabs" aria-label="作品分类">
            <button v-for="item in [{ key: 'all', label: '全部作品' }, { key: '新作', label: '全新作品' }, { key: '續作', label: '系列续作' }]"
              :key="item.key" type="button" :class="{ active: category === item.key }" @click="category = item.key">
              {{ item.label }}
            </button>
          </div>
          <label class="report-search"><span aria-hidden="true">⌕</span><input v-model="query" type="search" placeholder="搜索作品名称 / 日文原名" aria-label="搜索作品" /></label>
        </div>
        <div class="filter-bottom">
          <label>来源类型 <select v-model="origin"><option value="all">不限来源</option><option v-for="item in origins" :key="item" :value="item">{{ item }}</option></select></label>
          <label>题材分类 <select v-model="genre"><option value="all">全部题材</option><option v-for="item in genreOptions" :key="item" :value="item">{{ item }}</option></select></label>
          <label>排列顺序 <select v-model="sortMode"><option value="premiere">首播时间</option><option value="category">新作 / 续作</option><option value="title">作品名称</option></select></label>
          <div class="filter-count">显示 <strong>{{ visibleWorks.length }}</strong> / {{ works.length }} 部</div>
        </div>
      </div>

      <div v-if="visibleWorks.length" class="work-grid">
        <article v-for="(work, index) in visibleWorks" :key="work.id || work.title" class="work-card" :style="{ '--card-delay': `${Math.min(index, 12) * .045}s` }">
          <div class="work-header">
            <div class="work-art">
              <img v-if="work.cover" :src="work.cover" :alt="`${work.title}海报`" loading="lazy" />
              <div v-else class="cover-fallback">OCT<br />2026</div>
              <span class="work-number">{{ String(index + 1).padStart(2, '0') }}</span>
            </div>
            <div class="work-intro">
              <div class="work-overline"><span>{{ categoryLabel(work) }}</span> / 2026 OCTOBER</div>
              <h3>{{ work.title }}</h3>
              <p class="work-original">{{ work.originalTitle || '原名待公布' }}</p>
              <div class="work-date"><i />{{ work.schedule || '首播时间待定' }}</div>
              <div class="work-tags"><span v-for="tag in work.tags" :key="tag">{{ tag }}</span></div>
              <div class="work-story">
                <h4>故事概要 <span>STORY</span></h4>
                <p>{{ work.story || '官方尚未公布剧情简介，敬请期待。' }}</p>
              </div>
            </div>
          </div>
          <div class="work-roster">
            <section class="roster-section" aria-label="声优阵容">
              <h4>声优阵容 <span>CAST / {{ work.cast.length }}</span></h4>
              <div v-if="work.cast.length" class="roster-grid">
                <div v-for="person in work.cast.slice(0, castPreviewLimit)" :key="person.role" class="roster-entry" :title="`${person.role}：${person.names.join('、')} 饰`">
                  <span>{{ person.role }}：</span><strong>{{ person.names.join('、') }}</strong><span> 饰</span>
                </div>
              </div>
              <p v-else class="roster-empty">声优阵容待公布</p>
            </section>
            <section class="roster-section" aria-label="制作阵容">
              <h4>制作阵容 <span>STAFF / {{ work.staff.length }}</span></h4>
              <div v-if="work.staff.length" class="roster-grid">
                <div v-for="person in featuredStaff(work)" :key="person.role" class="roster-entry" :title="`${person.role}：${person.names.join('、')}`">
                  <span>{{ person.role }}：</span><strong>{{ person.names.join('、') }}</strong>
                </div>
              </div>
              <p v-else class="roster-empty">制作阵容待公布</p>
            </section>
          </div>
          <div class="work-footer"><span>{{ dateBadge(work) }} · {{ work.premiereAt ? '日本首播' : '档期待定' }}</span><a :href="work.sourceUrl" target="_blank" rel="noopener noreferrer">原始资料 ↗</a></div>
        </article>
      </div>
      <div v-else class="empty-state"><strong>没有找到符合条件的作品</strong><p>试试调整关键词或筛选条件。</p><button type="button" @click="resetFilters">清除筛选</button></div>

      <footer class="report-footer">
        <div><b>10月秋番报告</b><span>资料截至 {{ report.snapshotDate }}，开播时间及制作信息可能调整。</span></div>
        <a :href="report.source" target="_blank" rel="noopener noreferrer">资料来源：ACG Secrets.HK ↗</a>
      </footer>
    </div>

  </section>
</template>

<style scoped>
.season-preview-stage{position:relative;min-height:calc(100vh - 86px);overflow:hidden;background:linear-gradient(135deg,#fff9fb 0%,#fff 42%,#f2fcfe 100%);color:var(--ink-900)}
.preview-grid{position:absolute;inset:0;pointer-events:none;background-image:linear-gradient(rgba(239,90,150,.055) 1px,transparent 1px),linear-gradient(90deg,rgba(239,90,150,.055) 1px,transparent 1px);background-size:62px 62px;mask-image:linear-gradient(to bottom,#000,transparent 50%)}
.preview-halo{position:absolute;width:540px;height:540px;border-radius:50%;filter:blur(90px);pointer-events:none;opacity:.48}.preview-halo-pink{top:20px;left:-220px;background:#ffd7e7}.preview-halo-cyan{top:270px;right:-240px;background:#d2f7fb}
.preview-wrap{position:relative;max-width:1500px;margin:auto;padding:0 42px}
.preview-hero{min-height:570px;display:grid;grid-template-columns:1fr 1.02fr;align-items:center;gap:35px}
.hero-copy{position:relative;z-index:2;padding:65px 0}.hero-kicker,.section-kicker{display:flex;align-items:center;gap:11px;color:var(--pink-600);font-family:var(--font-mono);font-size:12px;font-weight:700;letter-spacing:2.5px}.kicker-dot{width:8px;height:8px;border:2px solid var(--pink-500);transform:rotate(45deg)}.kicker-line{width:26px;height:1px;background:var(--pink-300)}
.hero-overline{margin:30px 0 11px;color:var(--ink-600);font-size:17px;font-weight:700;letter-spacing:5px}.hero-copy h1{font-size:clamp(72px,6.5vw,112px);font-weight:900;line-height:1.05;letter-spacing:-5px}.hero-copy h1 em{color:var(--pink-500);font-style:normal}.hero-description{margin-top:30px;color:var(--ink-600);font-size:17px;line-height:1.9}.hero-actions{display:flex;align-items:center;gap:24px;margin-top:35px}.primary-action{display:inline-flex;align-items:center;justify-content:space-between;gap:40px;min-width:216px;padding:16px 22px;color:#fff;background:linear-gradient(120deg,#f34f91,#fb79a5);box-shadow:0 18px 28px rgba(239,74,139,.22);font-size:14px;font-weight:700;text-decoration:none;clip-path:polygon(0 0,calc(100% - 13px) 0,100% 13px,100% 100%,13px 100%,0 calc(100% - 13px));transition:transform .2s}.primary-action:hover{transform:translateY(-3px)}.hero-actions>span{color:var(--ink-400);font-family:var(--font-mono);font-size:10px;letter-spacing:1px}
.hero-art{position:relative;height:510px;perspective:900px}.art-orbit{position:absolute;border:1px solid rgba(238,63,134,.18);border-radius:50%;transform:rotate(-20deg)}.orbit-one{width:500px;height:390px;top:56px;left:8%}.orbit-two{width:360px;height:290px;top:95px;left:20%;border-color:rgba(73,214,233,.45)}.art-cover{position:absolute;overflow:hidden;background:#f7dfe9;border:7px solid #fff;box-shadow:0 22px 46px rgba(39,48,82,.22);border-radius:8px;transition:transform .3s}.art-cover img{width:100%;height:100%;object-fit:cover}.cover-1{z-index:3;top:68px;left:31%;width:245px;height:345px;transform:rotate(-6deg)}.cover-1:hover{transform:rotate(-3deg) translateY(-8px)}.cover-2{z-index:2;top:113px;left:3%;width:188px;height:273px;transform:rotate(-17deg)}.cover-3{z-index:1;top:85px;right:3%;width:190px;height:285px;transform:rotate(12deg)}.art-stamp{position:absolute;z-index:4;top:20px;right:10%;width:88px;height:88px;display:grid;place-content:center;color:#fff;background:#f3619b;border:5px solid #fff;box-shadow:0 8px 22px rgba(240,70,140,.25);border-radius:50%;font-family:var(--font-mono);font-size:13px;line-height:1.1;text-align:center;transform:rotate(11deg)}.art-stamp strong{font-size:28px}.art-caption{position:absolute;bottom:30px;right:3%;color:var(--ink-400);font-family:var(--font-mono);font-size:12px;line-height:1.6;letter-spacing:2px}
.season-facts{display:grid;grid-template-columns:repeat(3,1fr) 1.2fr;align-items:center;padding:24px 34px;background:var(--glass-strong);border:1px solid var(--line-soft);box-shadow:0 18px 55px rgba(50,70,110,.05);clip-path:polygon(0 0,calc(100% - 16px) 0,100% 16px,100% 100%,16px 100%,0 calc(100% - 16px))}.season-facts>div{display:grid;grid-template-columns:auto 1fr;column-gap:12px;align-items:baseline;border-right:1px solid var(--line-soft)}.season-facts strong{grid-row:span 2;color:var(--pink-500);font-family:var(--font-mono);font-size:39px;line-height:1}.season-facts span{font-size:13px;font-weight:700}.season-facts small{color:var(--ink-400);font-family:var(--font-mono);font-size:10px;letter-spacing:1px}.season-facts>p{padding-left:25px;color:var(--ink-600);font-size:12px;line-height:1.5}.season-facts>p b{color:var(--ink-900);font-family:var(--font-mono);font-size:16px}
.lineup-heading{display:flex;justify-content:space-between;align-items:end;margin:88px 0 30px}.section-kicker span{display:inline-grid;place-items:center;width:28px;height:28px;color:#fff;background:var(--pink-500);clip-path:polygon(50% 0,100% 50%,50% 100%,0 50%);font-size:11px}.lineup-heading h2{margin:15px 0 12px;font-size:39px;line-height:1.2}.lineup-heading h2 span{margin-left:12px;color:var(--pink-400);font-size:26px}.lineup-heading p:last-child{color:var(--ink-600);font-size:14px}.lineup-mark{color:var(--ink-300);font-family:var(--font-mono);font-size:17px;line-height:1.1;letter-spacing:4px;text-align:right}.lineup-mark strong{color:var(--pink-200);font-size:64px;font-weight:700;letter-spacing:-6px}
.filter-panel{padding:18px 22px;margin-bottom:24px;background:var(--glass-strong);border:1px solid var(--line-soft);box-shadow:0 12px 34px rgba(50,70,110,.045)}.filter-top,.filter-bottom{display:flex;align-items:center;gap:18px}.filter-top{justify-content:space-between;padding-bottom:18px;border-bottom:1px solid var(--line-soft)}.category-tabs{display:flex;gap:7px}.category-tabs button{padding:10px 18px;color:var(--ink-600);background:transparent;border:1px solid transparent;font-size:13px;font-weight:700}.category-tabs button.active{color:var(--pink-600);background:var(--pink-50);border-color:var(--line)}.report-search{min-width:285px;display:flex;align-items:center;gap:8px;padding:9px 13px;background:var(--glass);border:1px solid var(--line-soft)}.report-search span{color:var(--pink-500);font-size:26px;line-height:18px}.report-search input{width:100%;color:var(--ink-900);background:transparent;border:0;outline:none;font:inherit;font-size:13px}.report-search input::placeholder{color:var(--ink-400)}.filter-bottom{padding-top:16px}.filter-bottom label{display:flex;align-items:center;gap:10px;color:var(--ink-400);font-size:12px;font-weight:700}.filter-bottom select{min-width:126px;padding:8px 28px 8px 10px;color:var(--ink-700);background:var(--glass);border:1px solid var(--line-soft);font:inherit}.filter-count{margin-left:auto;color:var(--ink-400);font-size:12px}.filter-count strong{color:var(--pink-600);font-size:17px}
.work-grid { display:grid; grid-template-columns:minmax(0,1fr); gap:22px; }
.work-card { display:grid; grid-template-columns:minmax(0,57fr) minmax(0,43fr); min-width:0; overflow:hidden; background:var(--report-card); border:1px solid var(--report-border); box-shadow:var(--report-shadow); animation:card-enter .45s both; animation-delay:var(--card-delay); }
.work-header { display:grid; grid-template-columns:176px minmax(0,1fr); gap:20px; padding:22px; }
.work-art { position:relative; align-self:start; width:176px; aspect-ratio:2/3; overflow:hidden; background:linear-gradient(145deg,var(--pink-100),var(--cyan-100)); }
.work-art img { width:100%; height:100%; object-fit:cover; }
.cover-fallback { display:grid; place-items:center; height:100%; color:var(--pink-500); font-family:var(--font-mono); font-size:28px; text-align:center; }
.work-number { position:absolute; top:10px; left:10px; padding:5px 8px; color:#fff; background:rgba(32,40,62,.76); font-family:var(--font-mono); font-size:11px; }
.work-intro { min-width:0; }
.work-overline { color:var(--ink-400); font-family:var(--font-mono); font-size:10px; font-weight:700; letter-spacing:1.3px; }
.work-overline span { color:var(--pink-600); }
.work-intro h3 { margin:10px 0 5px; color:var(--ink-900); font-size:21px; line-height:1.4; overflow-wrap:anywhere; }
.work-original { color:var(--ink-400); font-size:12px; line-height:1.5; overflow-wrap:anywhere; }
.work-date { display:flex; align-items:start; gap:9px; margin-top:17px; color:var(--pink-600); font-size:12px; font-weight:700; line-height:1.5; }
.work-date i { flex:0 0 6px; width:6px; height:6px; margin-top:6px; background:var(--pink-500); transform:rotate(45deg); }
.work-tags { display:flex; flex-wrap:wrap; gap:5px; margin-top:13px; }
.work-tags span { padding:4px 7px; color:var(--ink-600); background:var(--report-inset); border:1px solid var(--report-border); font-size:12px; }
.work-story { margin-top:18px; padding-top:15px; border-top:1px solid var(--report-border); }
.work-story h4, .roster-section h4 { display:flex; align-items:baseline; gap:9px; color:var(--ink-900); font-size:14px; }
.work-story h4 span, .roster-section h4 span { color:var(--pink-500); font-family:var(--font-mono); font-size:10px; font-weight:600; letter-spacing:1.2px; }
.work-story p { margin-top:10px; color:var(--ink-600); font-size:12px; line-height:1.75; white-space:pre-line; }
.work-roster { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); align-content:start; gap:18px; min-width:0; padding:22px; border-left:1px solid var(--report-border); background:var(--report-inset); }
.roster-section { min-width:0; }
.roster-section + .roster-section { padding-left:18px; border-left:1px solid var(--report-border); }
.roster-grid { display:grid; margin-top:12px; }
.roster-entry { min-width:0; overflow:hidden; padding:8px 0; border-bottom:1px solid var(--report-border); color:var(--ink-400); font-size:12px; line-height:1.5; text-overflow:ellipsis; white-space:nowrap; }
.roster-entry strong { color:var(--ink-900); font-weight:700; }
.roster-empty { margin-top:12px; color:var(--ink-400); font-size:12px; }
.work-footer { grid-column:1 / -1; display:flex; justify-content:space-between; align-items:center; gap:16px; padding:13px 22px; color:var(--ink-400); background:var(--report-inset); border-top:1px solid var(--report-border); font-size:11px; }
.work-footer a { color:var(--pink-600); font-weight:700; text-decoration:none; white-space:nowrap; }
.work-footer a:hover { text-decoration:underline; }
.empty-state{padding:80px;text-align:center;background:var(--glass-strong);border:1px dashed var(--line)}.empty-state strong{font-size:19px}.empty-state p{margin:10px 0 20px;color:var(--ink-400)}.empty-state button{padding:10px 20px;color:#fff;background:var(--pink-500)}
.report-footer{display:flex;justify-content:space-between;align-items:center;gap:20px;margin-top:65px;padding:30px 0 48px;border-top:1px solid var(--line-soft);font-size:12px}.report-footer div{display:grid;gap:6px}.report-footer span{color:var(--ink-400)}.report-footer a{color:var(--pink-600);text-decoration:none}.report-footer a:hover{text-decoration:underline}
.season-preview-stage {
  --report-bg: linear-gradient(135deg,#fff9fb 0%,#fff 42%,#f2fcfe 100%);
  --report-grid: rgba(239,90,150,.055);
  --report-surface: rgba(255,255,255,.86);
  --report-card: rgba(255,255,255,.94);
  --report-input: rgba(255,255,255,.72);
  --report-inset: rgba(255,244,248,.72);
  --report-border: var(--line-soft);
  --report-art-border: #fff;
  --report-shadow: 0 12px 34px rgba(50,70,110,.055);
  --report-halo-opacity: .48;
  background: var(--report-bg);
}
.preview-grid { background-image:linear-gradient(var(--report-grid) 1px,transparent 1px),linear-gradient(90deg,var(--report-grid) 1px,transparent 1px); }
.preview-halo { opacity:var(--report-halo-opacity); }
.art-cover { border-color:var(--report-art-border); }
.season-facts,.filter-panel,.empty-state { background:var(--report-surface); border-color:var(--report-border); box-shadow:var(--report-shadow); }
.report-search,.filter-bottom select { background:var(--report-input); border-color:var(--report-border); }
@keyframes card-enter{from{opacity:0;transform:translateY(14px)}to{opacity:1;transform:translateY(0)}}
@media(max-width:1350px){.preview-wrap{padding:0 32px}.hero-copy h1{font-size:78px}.hero-art{transform:scale(.9);transform-origin:center right}.work-card{grid-template-columns:minmax(0,55fr) minmax(0,45fr)}.work-header{grid-template-columns:150px minmax(0,1fr);gap:16px}.work-art{width:150px}.work-roster{gap:12px;padding:20px}.roster-section + .roster-section{padding-left:12px}}
@media(prefers-reduced-motion:reduce){.work-card{animation:none}.art-cover,.primary-action{transition:none}}
</style>
