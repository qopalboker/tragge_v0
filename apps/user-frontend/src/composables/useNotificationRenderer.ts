import { t } from '@/i18n';
import type { InAppNotification } from '@/api/notifications';

interface RenderedNotification {
  title: string;
  message: string;
}

export function formatAmount(amountCents: number | undefined, currency: string | undefined): string {
  if (!amountCents) return '';
  if (currency === 'IRR') {
    const toman = Math.floor(amountCents / 10);
    return toman.toLocaleString('fa-IR') + ' تومان';
  }
  return '$' + (amountCents / 100).toFixed(2);
}

export function useNotificationRenderer() {
  function tWithFallback(key: string, fallbackKey: string, params?: Record<string, string | number>): string {
    const result = t(key, params);
    if (result === key) {
      return t(fallbackKey, params);
    }
    return result;
  }

  function renderNotification(n: InAppNotification): RenderedNotification {
    const meta = n.metadata || {};
    const contestName = String(meta.contest_name || '');
    const amount = formatAmount(
      meta.amount_cents as number | undefined,
      meta.currency as string | undefined,
    );

    switch (n.type) {
      case 'contest_joined':
        return {
          title: t('notif.contest_joined.title', { name: contestName }),
          message: t('notif.contest_joined.message'),
        };

      case 'contest_left':
        return {
          title: t('notif.contest_left.title', { name: contestName }),
          message: meta.refunded
            ? t('notif.contest_left.messageRefunded')
            : t('notif.contest_left.message'),
        };

      case 'prize_won':
        return {
          title: t('notif.prize_won.title', {
            amount: formatAmount(meta.prize_amount_cents as number, meta.currency as string),
            name: contestName,
          }),
          message: t('notif.prize_won.message', {
            rank: String(meta.rank || ''),
            amount: formatAmount(meta.prize_amount_cents as number, meta.currency as string),
          }),
        };

      case 'contest_completed':
        return {
          title: t('notif.contest_completed.title', { name: contestName }),
          message: t('notif.contest_completed.message', {
            rank: String(meta.final_rank || ''),
            total: String(meta.total_participants || ''),
          }),
        };

      case 'contest_cancelled':
        return {
          title: t('notif.contest_cancelled.title', { name: contestName }),
          message: meta.refund_amount_cents
            ? t('notif.contest_cancelled.messageRefunded', {
                reason: String(meta.reason || ''),
                amount: formatAmount(meta.refund_amount_cents as number, meta.currency as string),
              })
            : t('notif.contest_cancelled.message', { reason: String(meta.reason || '') }),
        };

      case 'contest_starting':
        return {
          title: t('notif.contest_starting.title', { name: contestName }),
          message: t('notif.contest_starting.message'),
        };

      case 'contest_ending':
        return {
          title: t('notif.contest_ending.title', { name: contestName }),
          message: t('notif.contest_ending.message'),
        };

      case 'contest_started':
        return {
          title: t('notif.contest_started.title'),
          message: t('notif.contest_started.message', { name: contestName }),
        };

      case 'registration_closed':
        return {
          title: t('notif.registration_closed.title'),
          message: t('notif.registration_closed.message', { name: contestName }),
        };

      case 'contest_paused':
        return {
          title: t('notif.contest_paused.title'),
          message: t('notif.contest_paused.message', { name: contestName }),
        };

      case 'contest_resumed':
        return {
          title: t('notif.contest_resumed.title'),
          message: t('notif.contest_resumed.message', { name: contestName }),
        };

      case 'deposit_confirmed':
        return {
          title: t('notif.deposit_confirmed.title', { amount }),
          message: t('notif.deposit_confirmed.message', {
            amount,
            provider: String(meta.provider || ''),
          }),
        };

      case 'deposit_failed':
        return {
          title: t('notif.deposit_failed.title'),
          message: t('notif.deposit_failed.message', {
            amount,
            provider: String(meta.provider || ''),
          }),
        };

      case 'withdrawal_update': {
        const status = String(meta.status || '');
        return {
          title: tWithFallback(`notif.withdrawal_update.title_${status}`, 'notif.withdrawal_update.title', { amount }),
          message: tWithFallback(`notif.withdrawal_update.message_${status}`, 'notif.withdrawal_update.message'),
        };
      }

      case 'kyc_update': {
        const status = String(meta.status || '');
        return {
          title: tWithFallback(`notif.kyc_update.title_${status}`, 'notif.kyc_update.title'),
          message: tWithFallback(`notif.kyc_update.message_${status}`, 'notif.kyc_update.message'),
        };
      }

      case 'ticket_reply': {
        const subject = String(meta.subject || '');
        return {
          title: t('notif.ticket_reply.title'),
          message: t('notif.ticket_reply.message', { subject }),
        };
      }

      case 'system':
        // System notifications: use stored title/message as-is (admin-written)
        return {
          title: n.title,
          message: n.message,
        };

      default:
        // Fallback: use stored title/message from backend
        return {
          title: n.title,
          message: n.message,
        };
    }
  }

  return { renderNotification, formatAmount };
}
