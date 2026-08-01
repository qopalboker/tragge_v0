import { computed } from 'vue';
import { t } from '@/i18n';

export interface DayInfo {
  date: Date;
  dayOfMonth: number;
  isToday: boolean;
  isCurrentMonth: boolean;
  isWeekend: boolean;
  dateKey: string; // YYYY-MM-DD format
}

export interface WeekInfo {
  weekNumber: number;
  days: DayInfo[];
}

/**
 * Composable for calendar date utilities
 */
export function useCalendar() {
  const weekDays = computed(() => [
    t('calendar.mon'),
    t('calendar.tue'),
    t('calendar.wed'),
    t('calendar.thu'),
    t('calendar.fri'),
    t('calendar.sat'),
    t('calendar.sun'),
  ]);

  const weekDaysFull = computed(() => [
    t('calendar.monday'),
    t('calendar.tuesday'),
    t('calendar.wednesday'),
    t('calendar.thursday'),
    t('calendar.friday'),
    t('calendar.saturday'),
    t('calendar.sunday'),
  ]);

  const months = computed(() => [
    t('calendar.january'),
    t('calendar.february'),
    t('calendar.march'),
    t('calendar.april'),
    t('calendar.may'),
    t('calendar.june'),
    t('calendar.july'),
    t('calendar.august'),
    t('calendar.september'),
    t('calendar.october'),
    t('calendar.november'),
    t('calendar.december'),
  ]);

  /**
   * Get the week days (Mon-Sun) for a given date
   */
  function getWeekDays(date: Date): DayInfo[] {
    const days: DayInfo[] = [];
    const today = new Date();
    today.setHours(0, 0, 0, 0);

    // Find Monday of the week
    const monday = new Date(date);
    const dayOfWeek = monday.getDay();
    const diff = dayOfWeek === 0 ? -6 : 1 - dayOfWeek;
    monday.setDate(monday.getDate() + diff);
    monday.setHours(0, 0, 0, 0);

    for (let i = 0; i < 7; i++) {
      const day = new Date(monday);
      day.setDate(monday.getDate() + i);

      days.push({
        date: day,
        dayOfMonth: day.getDate(),
        isToday: day.getTime() === today.getTime(),
        isCurrentMonth: day.getMonth() === date.getMonth(),
        isWeekend: i >= 5, // Saturday = 5, Sunday = 6
        dateKey: formatDateKey(day),
      });
    }

    return days;
  }

  /**
   * Get all weeks for a month view (includes days from prev/next months)
   */
  function getMonthWeeks(date: Date): WeekInfo[] {
    const weeks: WeekInfo[] = [];
    const year = date.getFullYear();
    const month = date.getMonth();
    const today = new Date();
    today.setHours(0, 0, 0, 0);

    // First day of the month
    const firstDay = new Date(year, month, 1);
    // Last day of the month
    const lastDay = new Date(year, month + 1, 0);

    // Find the Monday of the first week
    const current = new Date(firstDay);
    const dayOfWeek = current.getDay();
    const diff = dayOfWeek === 0 ? -6 : 1 - dayOfWeek;
    current.setDate(current.getDate() + diff);

    let weekNumber = 1;
    while (current <= lastDay || current.getDay() !== 1) {
      const week: DayInfo[] = [];

      for (let i = 0; i < 7; i++) {
        const day = new Date(current);

        week.push({
          date: day,
          dayOfMonth: day.getDate(),
          isToday: day.getTime() === today.getTime(),
          isCurrentMonth: day.getMonth() === month,
          isWeekend: i >= 5,
          dateKey: formatDateKey(day),
        });

        current.setDate(current.getDate() + 1);
      }

      weeks.push({ weekNumber, days: week });
      weekNumber++;

      // Break if we've passed the end of the month and completed the week
      if (current.getMonth() !== month && current.getDay() === 1) {
        break;
      }
    }

    return weeks;
  }

  /**
   * Format date as YYYY-MM-DD
   */
  function formatDateKey(date: Date): string {
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    return `${year}-${month}-${day}`;
  }

  /**
   * Format date range for display
   */
  function formatDateRange(from: Date, to: Date): string {
    const fromMonth = months.value[from.getMonth()];
    const toMonth = months.value[to.getMonth()];

    if (from.getMonth() === to.getMonth() && from.getFullYear() === to.getFullYear()) {
      return `${fromMonth} ${from.getDate()}-${to.getDate()}, ${from.getFullYear()}`;
    } else if (from.getFullYear() === to.getFullYear()) {
      return `${fromMonth} ${from.getDate()} - ${toMonth} ${to.getDate()}, ${from.getFullYear()}`;
    } else {
      return `${fromMonth} ${from.getDate()}, ${from.getFullYear()} - ${toMonth} ${to.getDate()}, ${to.getFullYear()}`;
    }
  }

  /**
   * Format month and year for display
   */
  function formatMonthYear(date: Date): string {
    return `${months.value[date.getMonth()]} ${date.getFullYear()}`;
  }

  /**
   * Format time from ISO string
   */
  function formatTime(isoString: string): string {
    const date = new Date(isoString);
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  /**
   * Format date for display
   */
  function formatDate(date: Date | string): string {
    const d = typeof date === 'string' ? new Date(date) : date;
    return `${months.value[d.getMonth()]} ${d.getDate()}, ${d.getFullYear()}`;
  }

  /**
   * Format duration in minutes to human readable string
   */
  function formatDuration(minutes: number): string {
    if (minutes < 60) {
      return `${minutes}m`;
    } else if (minutes < 1440) {
      const hours = Math.floor(minutes / 60);
      const mins = minutes % 60;
      return mins > 0 ? `${hours}h ${mins}m` : `${hours}h`;
    } else {
      const days = Math.floor(minutes / 1440);
      const hours = Math.floor((minutes % 1440) / 60);
      return hours > 0 ? `${days}d ${hours}h` : `${days}d`;
    }
  }

  /**
   * Check if two dates are the same day
   */
  function isSameDay(date1: Date, date2: Date): boolean {
    return (
      date1.getFullYear() === date2.getFullYear() &&
      date1.getMonth() === date2.getMonth() &&
      date1.getDate() === date2.getDate()
    );
  }

  /**
   * Check if date is today
   */
  function isToday(date: Date): boolean {
    return isSameDay(date, new Date());
  }

  /**
   * Check if date is in the past
   */
  function isPast(date: Date): boolean {
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const checkDate = new Date(date);
    checkDate.setHours(0, 0, 0, 0);
    return checkDate < today;
  }

  /**
   * Get relative day label (Today, Tomorrow, Yesterday, or date)
   */
  function getRelativeDay(date: Date): string {
    const today = new Date();
    today.setHours(0, 0, 0, 0);

    const checkDate = new Date(date);
    checkDate.setHours(0, 0, 0, 0);

    const diffDays = Math.round((checkDate.getTime() - today.getTime()) / (1000 * 60 * 60 * 24));

    if (diffDays === 0) return t('calendar.today');
    if (diffDays === 1) return t('calendar.tomorrow');
    if (diffDays === -1) return t('calendar.yesterday');

    return formatDate(date);
  }

  /**
   * Generate ICS calendar event
   */
  function generateICS(event: {
    title: string;
    description: string;
    startTime: string;
    endTime: string;
    location?: string;
  }): string {
    const formatICSDate = (isoString: string) => {
      return new Date(isoString)
        .toISOString()
        .replace(/[-:]/g, '')
        .split('.')[0] + 'Z';
    };

    const uid = `${Date.now()}-${Math.random().toString(36).substr(2, 9)}@tragge.com`;

    return [
      'BEGIN:VCALENDAR',
      'VERSION:2.0',
      'PRODID:-//Tragge//Tournament Calendar//EN',
      'CALSCALE:GREGORIAN',
      'METHOD:PUBLISH',
      'BEGIN:VEVENT',
      `UID:${uid}`,
      `DTSTAMP:${formatICSDate(new Date().toISOString())}`,
      `DTSTART:${formatICSDate(event.startTime)}`,
      `DTEND:${formatICSDate(event.endTime)}`,
      `SUMMARY:${event.title}`,
      `DESCRIPTION:${event.description.replace(/\n/g, '\\n')}`,
      event.location ? `LOCATION:${event.location}` : '',
      'END:VEVENT',
      'END:VCALENDAR',
    ]
      .filter(Boolean)
      .join('\r\n');
  }

  /**
   * Download ICS file
   */
  function downloadICS(event: {
    title: string;
    description: string;
    startTime: string;
    endTime: string;
  }): void {
    const icsContent = generateICS(event);
    const blob = new Blob([icsContent], { type: 'text/calendar;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `${event.title.replace(/[^a-z0-9]/gi, '_')}.ics`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  }

  return {
    weekDays,
    weekDaysFull,
    months,
    getWeekDays,
    getMonthWeeks,
    formatDateKey,
    formatDateRange,
    formatMonthYear,
    formatTime,
    formatDate,
    formatDuration,
    isSameDay,
    isToday,
    isPast,
    getRelativeDay,
    generateICS,
    downloadICS,
  };
}
