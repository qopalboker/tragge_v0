<script setup lang="ts">
import { ref, computed, watch, reactive, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { t, direction } from '@/i18n';
import { api } from '@/api/index';
import type { CalendarContest, CalendarGroup, CalendarGroupedResponse } from '@/api/calendar';
import { useCalendar } from '@/composables/useCalendar';

const router = useRouter();
const { formatDateKey, formatDuration, months } = useCalendar();

const emit = defineEmits<{
  joinClick: [contest: CalendarContest];
}>();

// --------------- State ---------------
const currentDate = ref(new Date());
const selectedDateKey = ref<string | null>(null);
const loading = ref(false);
const error = ref<string | null>(null);
const cache = reactive<Record<string, CalendarGroup[]>>({});

// --------------- Computed ---------------
const isRtl = computed(() => direction.value === 'rtl');
const currentYear = computed(() => currentDate.value.getFullYear());
const currentMonth = computed(() => currentDate.value.getMonth());
const monthLabel = computed(() => `${months.value[currentMonth.value]} ${currentYear.value}`);

const monthCacheKey = computed(
  () => `${currentYear.value}-${String(currentMonth.value + 1).padStart(2, '0')}`
);

const monthStartStr = computed(
  () => `${currentYear.value}-${String(currentMonth.value + 1).padStart(2, '0')}-01`
);

const monthEndStr = computed(() => {
  const lastDay = new Date(currentYear.value, currentMonth.value + 1, 0);
  return formatDateKey(lastDay);
});

const hasCachedData = computed(() => monthCacheKey.value in cache);

// Week day headers: Sat-Fri for RTL, Mon-Sun for LTR
const weekDayHeaders = computed(() => {
  if (isRtl.value) {
    return [
      t('calendar.sat'), t('calendar.sun'), t('calendar.mon'),
      t('calendar.tue'), t('calendar.wed'), t('calendar.thu'), t('calendar.fri'),
    ];
  }
  return [
    t('calendar.mon'), t('calendar.tue'), t('calendar.wed'),
    t('calendar.thu'), t('calendar.fri'), t('calendar.sat'), t('calendar.sun'),
  ];
});

// --------------- Calendar Grid ---------------
interface CalendarDay {
  date: Date;
  dayOfMonth: number;
  dateKey: string;
  isCurrentMonth: boolean;
  isToday: boolean;
}

const calendarWeeks = computed(() => {
  const year = currentYear.value;
  const month = currentMonth.value;
  const today = new Date();
  today.setHours(0, 0, 0, 0);

  const firstDay = new Date(year, month, 1);
  // Saturday (6) for RTL, Monday (1) for LTR
  const startDayOfWeek = isRtl.value ? 6 : 1;

  // Find the start date of the grid
  const start = new Date(firstDay);
  let dow = start.getDay();
  let diff = dow - startDayOfWeek;
  if (diff < 0) diff += 7;
  start.setDate(start.getDate() - diff);

  const weeks: CalendarDay[][] = [];
  const cursor = new Date(start);

  for (let w = 0; w < 6; w++) {
    const week: CalendarDay[] = [];
    for (let d = 0; d < 7; d++) {
      const day = new Date(cursor);
      week.push({
        date: day,
        dayOfMonth: day.getDate(),
        dateKey: formatDateKey(day),
        isCurrentMonth: day.getMonth() === month,
        isToday: day.getTime() === today.getTime(),
      });
      cursor.setDate(cursor.getDate() + 1);
    }
    weeks.push(week);

    // Stop if we've gone past the month and are at the start of a new week
    if (cursor.getMonth() !== month && cursor.getDay() === startDayOfWeek) {
      break;
    }
  }

  return weeks;
});

// --------------- Contest Data ---------------
const contestsByDate = computed(() => {
  const groups = cache[monthCacheKey.value];
  if (!groups) return new Map<string, CalendarContest[]>();
  const map = new Map<string, CalendarContest[]>();
  for (const group of groups) {
    map.set(group.key, group.contests);
  }
  return map;
});

function getDayContests(dateKey: string): CalendarContest[] {
  return contestsByDate.value.get(dateKey) || [];
}

const selectedDayContests = computed(() => {
  if (!selectedDateKey.value) return [];
  return getDayContests(selectedDateKey.value);
});

const selectedWeekIndex = computed(() => {
  if (!selectedDateKey.value) return -1;
  return calendarWeeks.value.findIndex(week =>
    week.some(day => day.dateKey === selectedDateKey.value)
  );
});

const selectedDayLabel = computed(() => {
  if (!selectedDateKey.value) return '';
  const groups = cache[monthCacheKey.value];
  const group = groups?.find(g => g.key === selectedDateKey.value);
  if (group) return group.label;
  const d = new Date(selectedDateKey.value + 'T00:00:00');
  return d.toLocaleDateString(isRtl.value ? 'fa' : 'en', {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
  });
});

// --------------- API ---------------
async function fetchMonth() {
  const key = monthCacheKey.value;
  if (key in cache) return;

  loading.value = true;
  error.value = null;

  try {
    const response = await api.get<CalendarGroupedResponse>('/api/user/contests/calendar', {
      params: {
        from: monthStartStr.value,
        to: monthEndStr.value,
        group_by: 'day',
      },
    });
    cache[key] = response.data.groups || [];
  } catch {
    error.value = t('calendar.loadError');
  } finally {
    loading.value = false;
  }
}

// --------------- Navigation ---------------
function prevMonth() {
  const d = new Date(currentDate.value);
  d.setMonth(d.getMonth() - 1);
  currentDate.value = d;
  selectedDateKey.value = null;
}

function nextMonth() {
  const d = new Date(currentDate.value);
  d.setMonth(d.getMonth() + 1);
  currentDate.value = d;
  selectedDateKey.value = null;
}

function goToToday() {
  currentDate.value = new Date();
  selectedDateKey.value = formatDateKey(new Date());
}

function selectDay(day: CalendarDay) {
  selectedDateKey.value = selectedDateKey.value === day.dateKey ? null : day.dateKey;
}

// --------------- Dot Indicators ---------------
interface DotInfo {
  type: 'free' | 'paid' | 'live';
  pulse: boolean;
}

function getDayDots(contests: CalendarContest[]): DotInfo[] {
  const dots: DotInfo[] = [];
  let hasLive = false;
  let hasFree = false;
  let hasPaid = false;

  for (const c of contests) {
    if (c.status === 'running') hasLive = true;
    if (c.entry_fee === 0) hasFree = true;
    if (c.entry_fee > 0) hasPaid = true;
  }

  if (hasLive) dots.push({ type: 'live', pulse: true });
  if (hasFree) dots.push({ type: 'free', pulse: false });
  if (hasPaid) dots.push({ type: 'paid', pulse: false });

  return dots;
}

// --------------- Detail Panel Helpers ---------------
function formatContestTime(iso: string): string {
  return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function formatEntryFee(fee: number): string {
  if (fee === 0) return t('contests.free');
  return `$${fee.toFixed(2)}`;
}

function formatParticipants(p: { current: number; max?: number }): string {
  return p.max ? `${p.current}/${p.max}` : String(p.current);
}

function getAssetClassLabel(assetClass: string): string {
  return t(`calendar.${assetClass}`);
}

function navigateToContest(id: string) {
  router.push(`/user/contests/${id}`);
}

function handleJoinClick(contest: CalendarContest) {
  emit('joinClick', contest);
}

// --------------- Lifecycle ---------------
watch(currentDate, () => fetchMonth());
onMounted(() => fetchMonth());
</script>

<template>
  <div class="contest-calendar">
    <!-- Header -->
    <div class="calendar-header">
      <div class="header-top">
        <h2 class="calendar-title">{{ t('calendar.title') }}</h2>
        <button class="btn-today" @click="goToToday">{{ t('calendar.today') }}</button>
      </div>
      <div class="month-nav">
        <button class="nav-btn" @click="prevMonth" :aria-label="t('calendar.prevMonth')">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="15 18 9 12 15 6" />
          </svg>
        </button>
        <span class="month-label">{{ monthLabel }}</span>
        <button class="nav-btn" @click="nextMonth" :aria-label="t('calendar.nextMonth')">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="9 18 15 12 9 6" />
          </svg>
        </button>
      </div>
    </div>

    <!-- Loading skeleton -->
    <div v-if="loading && !hasCachedData" class="calendar-skeleton">
      <div class="skeleton-weekday-row">
        <div v-for="n in 7" :key="n" class="skeleton-weekday"></div>
      </div>
      <div v-for="w in 5" :key="w" class="skeleton-week">
        <div v-for="d in 7" :key="d" class="skeleton-day">
          <div class="skeleton-day-num"></div>
          <div class="skeleton-dots"></div>
        </div>
      </div>
    </div>

    <!-- Error state -->
    <div v-else-if="error && !hasCachedData" class="calendar-error">
      <p>{{ error }}</p>
      <button class="btn btn-secondary btn-sm" @click="fetchMonth">{{ t('common.retry') }}</button>
    </div>

    <!-- Calendar grid -->
    <div v-else class="calendar-body">
      <!-- Weekday headers -->
      <div class="weekday-row">
        <div v-for="day in weekDayHeaders" :key="day" class="weekday-cell">
          {{ day }}
        </div>
      </div>

      <!-- Week rows with detail panel insertion -->
      <template v-for="(week, weekIdx) in calendarWeeks" :key="weekIdx">
        <div class="week-row">
          <div
            v-for="day in week"
            :key="day.dateKey"
            :class="[
              'day-cell',
              {
                'is-today': day.isToday,
                'is-other-month': !day.isCurrentMonth,
                'is-selected': selectedDateKey === day.dateKey,
                'has-contests': getDayContests(day.dateKey).length > 0,
              },
            ]"
            @click="selectDay(day)"
          >
            <span :class="['day-number', { today: day.isToday }]">
              {{ day.dayOfMonth }}
            </span>

            <div v-if="getDayContests(day.dateKey).length > 0" class="day-dots">
              <span
                v-for="(dot, i) in getDayDots(getDayContests(day.dateKey))"
                :key="i"
                :class="['dot', `dot-${dot.type}`, { pulse: dot.pulse }]"
              ></span>
              <span
                v-if="getDayContests(day.dateKey).length > 3"
                class="dot-count"
              >
                +{{ getDayContests(day.dateKey).length }}
              </span>
            </div>
          </div>
        </div>

        <!-- Detail panel: slide down below the week containing the selected day -->
        <Transition name="slide-down">
          <div
            v-if="selectedDateKey && selectedWeekIndex === weekIdx"
            class="detail-panel"
          >
            <div class="detail-panel-header">
              <span class="detail-date-label">{{ selectedDayLabel }}</span>
              <button class="detail-close" @click="selectedDateKey = null">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <line x1="18" y1="6" x2="6" y2="18" />
                  <line x1="6" y1="6" x2="18" y2="18" />
                </svg>
              </button>
            </div>

            <div v-if="selectedDayContests.length === 0" class="detail-empty">
              {{ t('calendar.noContestsOnDay') }}
            </div>

            <div v-else class="detail-contest-list">
              <div
                v-for="contest in selectedDayContests"
                :key="contest.id"
                class="mini-card"
              >
                <div class="mini-card-top">
                  <a
                    class="mini-card-name"
                    href="#"
                    @click.prevent="navigateToContest(contest.id)"
                  >
                    {{ contest.name }}
                  </a>
                  <span class="mini-card-time">{{ formatContestTime(contest.starts_at) }}</span>
                </div>

                <div class="mini-card-badges">
                  <span :class="['mc-badge', `mc-badge-${contest.asset_class}`]">
                    {{ getAssetClassLabel(contest.asset_class) }}
                  </span>
                  <span class="mc-badge mc-badge-duration">
                    {{ formatDuration(contest.duration_minutes) }}
                  </span>
                  <span
                    :class="[
                      'mc-badge',
                      contest.entry_fee === 0 ? 'mc-badge-free' : 'mc-badge-paid',
                    ]"
                  >
                    {{ formatEntryFee(contest.entry_fee) }}
                  </span>
                </div>

                <div class="mini-card-bottom">
                  <span class="mini-card-participants">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
                      <circle cx="9" cy="7" r="4" />
                    </svg>
                    {{ formatParticipants(contest.participants) }}
                  </span>

                  <button
                    v-if="!contest.user_registered && contest.status === 'registration_open'"
                    class="btn btn-primary btn-sm mc-join-btn"
                    @click.stop="handleJoinClick(contest)"
                  >
                    {{ t('contests.joinNow') }}
                  </button>
                  <span
                    v-else-if="contest.user_registered"
                    class="mc-joined-badge"
                  >
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                      <polyline points="20 6 9 17 4 12" />
                    </svg>
                    {{ t('contests.joined') }}
                  </span>
                </div>
              </div>
            </div>
          </div>
        </Transition>
      </template>
    </div>
  </div>
</template>

<style scoped>
/* ==================== Container ==================== */
.contest-calendar {
  display: flex;
  flex-direction: column;
  background: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  overflow: hidden;
}

/* ==================== Header ==================== */
.calendar-header {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  padding: var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
}

.header-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.calendar-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.btn-today {
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
  font-weight: 500;
  color: var(--color-primary);
  background: transparent;
  border: 1px solid var(--color-primary);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: background var(--transition-fast, 0.15s), color var(--transition-fast, 0.15s);
}

.btn-today:hover {
  background: var(--color-primary);
  color: white;
}

.month-nav {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-md);
}

.nav-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  background: transparent;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: background var(--transition-fast, 0.15s), color var(--transition-fast, 0.15s);
}

.nav-btn:hover {
  background: var(--color-bg-secondary);
  color: var(--color-text-primary);
}

[dir="rtl"] .nav-btn svg {
  transform: scaleX(-1);
}

.month-label {
  font-size: var(--font-size-md);
  font-weight: 600;
  color: var(--color-text-primary);
  min-width: 160px;
  text-align: center;
}

/* ==================== Calendar Body ==================== */
.calendar-body {
  display: flex;
  flex-direction: column;
}

/* Weekday header row */
.weekday-row {
  display: grid;
  grid-template-columns: repeat(7, 1fr);
  border-bottom: 1px solid var(--color-border);
}

.weekday-cell {
  padding: var(--spacing-sm);
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--color-text-muted);
  text-align: center;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

/* Week row */
.week-row {
  display: grid;
  grid-template-columns: repeat(7, 1fr);
  border-bottom: 1px solid var(--color-border-light, var(--color-border));
}

.week-row:last-of-type {
  border-bottom: none;
}

/* ==================== Day Cell ==================== */
.day-cell {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-xs);
  min-height: 72px;
  padding: var(--spacing-sm);
  background: var(--color-bg-primary);
  border-right: 1px solid var(--color-border-light, var(--color-border));
  cursor: pointer;
  transition: background var(--transition-fast, 0.15s);
}

.day-cell:last-child {
  border-right: none;
}

.day-cell:hover {
  background: var(--color-bg-secondary);
}

.day-cell.is-other-month {
  opacity: 0.4;
}

.day-cell.is-today {
  border: 2px solid var(--color-primary);
  border-radius: 0;
}

.day-cell.is-selected {
  background: rgba(59, 130, 246, 0.08);
}

.day-cell.has-contests {
  cursor: pointer;
}

/* Day number */
.day-number {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
  line-height: 1;
}

.day-number.today {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  background: var(--color-primary);
  color: white;
  border-radius: 50%;
  font-weight: 600;
}

/* Dot indicators */
.day-dots {
  display: flex;
  align-items: center;
  gap: 3px;
  flex-wrap: wrap;
  justify-content: center;
}

.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.dot-free {
  background: #10b981;
}

.dot-paid {
  background: #3b82f6;
}

.dot-live {
  background: #ef4444;
}

.dot.pulse {
  animation: dot-pulse 1.5s ease-in-out infinite;
}

@keyframes dot-pulse {
  0%, 100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.6;
    transform: scale(1.3);
  }
}

.dot-count {
  font-size: 10px;
  font-weight: 600;
  color: var(--color-text-secondary);
  line-height: 1;
}

/* ==================== Detail Panel ==================== */
.detail-panel {
  border-bottom: 1px solid var(--color-border);
  background: var(--color-bg-secondary);
  padding: var(--spacing-md) var(--spacing-lg);
  overflow: hidden;
}

.detail-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--spacing-md);
}

.detail-date-label {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-primary);
}

.detail-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  background: transparent;
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-text-muted);
  cursor: pointer;
  transition: background var(--transition-fast, 0.15s);
}

.detail-close:hover {
  background: var(--color-bg-tertiary);
  color: var(--color-text-primary);
}

.detail-empty {
  text-align: center;
  padding: var(--spacing-lg);
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
}

.detail-contest-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

/* ==================== Mini Card ==================== */
.mini-card {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  padding: var(--spacing-md);
  background: var(--color-bg-primary);
  border: 1px solid var(--color-border-light, var(--color-border));
  border-radius: var(--radius-md);
  transition: border-color var(--transition-fast, 0.15s);
}

.mini-card:hover {
  border-color: var(--color-border);
}

.mini-card-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-sm);
}

.mini-card-name {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-primary);
  text-decoration: none;
  cursor: pointer;
}

.mini-card-name:hover {
  color: var(--color-primary);
}

.mini-card-time {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  white-space: nowrap;
}

/* Mini card badges */
.mini-card-badges {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-xs);
}

.mc-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px var(--spacing-sm);
  font-size: var(--font-size-xs);
  font-weight: 500;
  border-radius: var(--radius-md);
  white-space: nowrap;
}

.mc-badge-crypto {
  background: #fef3c7;
  color: #d97706;
}

.mc-badge-forex {
  background: #d1fae5;
  color: #059669;
}

.mc-badge-stocks {
  background: #dbeafe;
  color: #2563eb;
}

.mc-badge-mixed {
  background: #ede9fe;
  color: #7c3aed;
}

.mc-badge-duration {
  background: var(--color-bg-tertiary);
  color: var(--color-text-secondary);
}

.mc-badge-free {
  background: #ecfdf5;
  color: #059669;
}

.mc-badge-paid {
  background: #dbeafe;
  color: #2563eb;
}

/* Mini card footer */
.mini-card-bottom {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-sm);
}

.mini-card-participants {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

.mc-join-btn {
  padding: var(--spacing-xs) var(--spacing-md);
  font-size: var(--font-size-xs);
  font-weight: 600;
}

.mc-joined-badge {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  background: #ecfdf5;
  color: #059669;
  border-radius: var(--radius-md);
  font-size: var(--font-size-xs);
  font-weight: 500;
}

/* ==================== Slide-Down Transition ==================== */
.slide-down-enter-active,
.slide-down-leave-active {
  transition: max-height 0.3s ease, opacity 0.3s ease;
  overflow: hidden;
}

.slide-down-enter-from,
.slide-down-leave-to {
  max-height: 0;
  opacity: 0;
  padding-top: 0;
  padding-bottom: 0;
}

.slide-down-enter-to,
.slide-down-leave-from {
  max-height: 600px;
  opacity: 1;
}

/* ==================== Loading Skeleton ==================== */
.calendar-skeleton {
  padding: var(--spacing-md);
}

.skeleton-weekday-row {
  display: grid;
  grid-template-columns: repeat(7, 1fr);
  gap: var(--spacing-xs);
  margin-bottom: var(--spacing-md);
}

.skeleton-weekday {
  height: 16px;
  background: var(--color-bg-tertiary);
  border-radius: var(--radius-sm);
  animation: skeleton-pulse 1.5s ease-in-out infinite;
}

.skeleton-week {
  display: grid;
  grid-template-columns: repeat(7, 1fr);
  gap: var(--spacing-xs);
  margin-bottom: var(--spacing-xs);
}

.skeleton-day {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-sm);
  min-height: 72px;
}

.skeleton-day-num {
  width: 24px;
  height: 16px;
  background: var(--color-bg-tertiary);
  border-radius: var(--radius-sm);
  animation: skeleton-pulse 1.5s ease-in-out infinite;
}

.skeleton-dots {
  display: flex;
  gap: 3px;
}

.skeleton-dots::before,
.skeleton-dots::after {
  content: '';
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--color-bg-tertiary);
  animation: skeleton-pulse 1.5s ease-in-out infinite;
}

@keyframes skeleton-pulse {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.4;
  }
}

/* ==================== Error State ==================== */
.calendar-error {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-2xl);
  text-align: center;
}

.calendar-error p {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin: 0;
}

/* ==================== RTL Support ==================== */
[dir="rtl"] .day-cell {
  border-right: none;
  border-left: 1px solid var(--color-border-light, var(--color-border));
}

[dir="rtl"] .day-cell:last-child {
  border-left: none;
}

/* ==================== Mobile Responsive ==================== */
@media (max-width: 639px) {
  .calendar-header {
    padding: var(--spacing-md);
  }

  .month-label {
    font-size: var(--font-size-sm);
    min-width: 120px;
  }

  .day-cell {
    min-height: 52px;
    padding: var(--spacing-xs);
  }

  .day-number {
    font-size: var(--font-size-xs);
  }

  .day-number.today {
    width: 20px;
    height: 20px;
    font-size: var(--font-size-xs);
  }

  .dot {
    width: 6px;
    height: 6px;
  }

  .dot-count {
    font-size: 9px;
  }

  .detail-panel {
    padding: var(--spacing-sm) var(--spacing-md);
  }

  .mini-card {
    padding: var(--spacing-sm);
  }

  .skeleton-day {
    min-height: 52px;
  }
}

/* ==================== Dark Theme ==================== */
.dark .mc-badge-crypto {
  background: rgba(245, 158, 11, 0.15);
}

.dark .mc-badge-forex {
  background: rgba(16, 185, 129, 0.15);
}

.dark .mc-badge-stocks {
  background: rgba(59, 130, 246, 0.15);
}

.dark .mc-badge-mixed {
  background: rgba(139, 92, 246, 0.15);
}

.dark .mc-badge-free {
  background: rgba(16, 185, 129, 0.15);
}

.dark .mc-badge-paid {
  background: rgba(59, 130, 246, 0.15);
}

.dark .mc-joined-badge {
  background: rgba(16, 185, 129, 0.15);
}

.dark .day-cell.is-selected {
  background: rgba(59, 130, 246, 0.12);
}
</style>
