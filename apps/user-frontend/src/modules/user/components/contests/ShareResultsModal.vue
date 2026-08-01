<script setup lang="ts">
import { ref, computed } from 'vue';
import { t } from '@/i18n';
import { useToast } from '@/composables/useToast';

interface ShareData {
  contestId: string;
  contestName: string;
  rank: number;
  totalParticipants: number;
  pnlPercent: number;
  prizeCents?: number;
}

const props = defineProps<{
  show: boolean;
  data: ShareData;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
}>();

const toast = useToast();
const copying = ref(false);

// Generate share URL
const shareUrl = computed(() => {
  return `${window.location.origin}/user/contests/${props.data.contestId}/results`;
});

// Generate share text
const shareText = computed(() => {
  const { rank, totalParticipants, pnlPercent, prizeCents, contestName } = props.data;
  const prize = prizeCents ? `$${(prizeCents / 100).toFixed(2)}` : '';
  const sign = pnlPercent >= 0 ? '+' : '';

  if (rank === 1) {
    return t('share.textFirst', {
      contest: contestName,
      pnl: `${sign}${pnlPercent.toFixed(2)}%`,
      prize,
    });
  }

  if (rank <= 3) {
    return t('share.textPodium', {
      rank,
      contest: contestName,
      pnl: `${sign}${pnlPercent.toFixed(2)}%`,
      prize,
    });
  }

  return t('share.textGeneral', {
    rank,
    total: totalParticipants,
    contest: contestName,
    pnl: `${sign}${pnlPercent.toFixed(2)}%`,
  });
});

// Social share URLs
const twitterUrl = computed(() => {
  const text = encodeURIComponent(shareText.value);
  const url = encodeURIComponent(shareUrl.value);
  return `https://twitter.com/intent/tweet?text=${text}&url=${url}`;
});

const telegramUrl = computed(() => {
  const text = encodeURIComponent(shareText.value);
  const url = encodeURIComponent(shareUrl.value);
  return `https://t.me/share/url?url=${url}&text=${text}`;
});

const whatsappUrl = computed(() => {
  const text = encodeURIComponent(`${shareText.value}\n${shareUrl.value}`);
  return `https://wa.me/?text=${text}`;
});

// Copy link to clipboard
async function copyLink(): Promise<void> {
  if (copying.value) return;

  copying.value = true;
  try {
    await navigator.clipboard.writeText(shareUrl.value);
    toast.success(t('share.linkCopied'));
  } catch {
    toast.error(t('share.copyFailed'));
  } finally {
    setTimeout(() => {
      copying.value = false;
    }, 1500);
  }
}

// Open share window
function openShare(url: string): void {
  window.open(url, '_blank', 'width=600,height=400');
}

// Close modal
function handleClose(): void {
  emit('close');
}

// Handle backdrop click
function handleBackdropClick(event: MouseEvent): void {
  if (event.target === event.currentTarget) {
    handleClose();
  }
}
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="show" class="modal-backdrop" @click="handleBackdropClick">
        <div class="modal-container">
          <div class="modal-header">
            <h3 class="modal-title">{{ t('share.title') }}</h3>
            <button class="close-btn" @click="handleClose">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="18" y1="6" x2="6" y2="18" />
                <line x1="6" y1="6" x2="18" y2="18" />
              </svg>
            </button>
          </div>

          <div class="modal-body">
            <!-- Preview Card -->
            <div class="preview-card">
              <div class="preview-header">
                <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M6 9H4.5a2.5 2.5 0 0 1 0-5H6M18 9h1.5a2.5 2.5 0 0 0 0-5H18M4 22h16M10 14.66V17c0 .55-.47.98-.97 1.21C7.85 18.75 7 20.24 7 22M14 14.66V17c0 .55.47.98.97 1.21C16.15 18.75 17 20.24 17 22M18 2H6v7a6 6 0 0 0 12 0V2Z" />
                </svg>
                <span class="preview-badge">{{ t('share.tradingResults') }}</span>
              </div>
              <h4 class="preview-contest">{{ data.contestName }}</h4>
              <div class="preview-stats">
                <div class="preview-stat">
                  <span class="stat-label">{{ t('share.rank') }}</span>
                  <span class="stat-value rank">#{{ data.rank }}<span class="of">/ {{ data.totalParticipants }}</span></span>
                </div>
                <div class="preview-stat">
                  <span class="stat-label">{{ t('share.pnl') }}</span>
                  <span class="stat-value" :class="{ positive: data.pnlPercent >= 0, negative: data.pnlPercent < 0 }">
                    {{ data.pnlPercent >= 0 ? '+' : '' }}{{ data.pnlPercent.toFixed(2) }}%
                  </span>
                </div>
                <div v-if="data.prizeCents" class="preview-stat">
                  <span class="stat-label">{{ t('share.prize') }}</span>
                  <span class="stat-value prize">${{ (data.prizeCents / 100).toFixed(2) }}</span>
                </div>
              </div>
            </div>

            <!-- Share via section -->
            <div class="share-section">
              <span class="section-label">{{ t('share.shareVia') }}</span>
              <div class="social-buttons">
                <button class="social-btn twitter" @click="openShare(twitterUrl)">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z"/>
                  </svg>
                  <span>Twitter</span>
                </button>

                <button class="social-btn telegram" @click="openShare(telegramUrl)">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z"/>
                  </svg>
                  <span>Telegram</span>
                </button>

                <button class="social-btn whatsapp" @click="openShare(whatsappUrl)">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M17.472 14.382c-.297-.149-1.758-.867-2.03-.967-.273-.099-.471-.148-.67.15-.197.297-.767.966-.94 1.164-.173.199-.347.223-.644.075-.297-.15-1.255-.463-2.39-1.475-.883-.788-1.48-1.761-1.653-2.059-.173-.297-.018-.458.13-.606.134-.133.298-.347.446-.52.149-.174.198-.298.298-.497.099-.198.05-.371-.025-.52-.075-.149-.669-1.612-.916-2.207-.242-.579-.487-.5-.669-.51-.173-.008-.371-.01-.57-.01-.198 0-.52.074-.792.372-.272.297-1.04 1.016-1.04 2.479 0 1.462 1.065 2.875 1.213 3.074.149.198 2.096 3.2 5.077 4.487.709.306 1.262.489 1.694.625.712.227 1.36.195 1.871.118.571-.085 1.758-.719 2.006-1.413.248-.694.248-1.289.173-1.413-.074-.124-.272-.198-.57-.347m-5.421 7.403h-.004a9.87 9.87 0 01-5.031-1.378l-.361-.214-3.741.982.998-3.648-.235-.374a9.86 9.86 0 01-1.51-5.26c.001-5.45 4.436-9.884 9.888-9.884 2.64 0 5.122 1.03 6.988 2.898a9.825 9.825 0 012.893 6.994c-.003 5.45-4.437 9.884-9.885 9.884m8.413-18.297A11.815 11.815 0 0012.05 0C5.495 0 .16 5.335.157 11.892c0 2.096.547 4.142 1.588 5.945L.057 24l6.305-1.654a11.882 11.882 0 005.683 1.448h.005c6.554 0 11.89-5.335 11.893-11.893a11.821 11.821 0 00-3.48-8.413z"/>
                  </svg>
                  <span>WhatsApp</span>
                </button>
              </div>
            </div>

            <!-- Copy Link -->
            <div class="copy-section">
              <span class="section-label">{{ t('share.orCopyLink') }}</span>
              <div class="copy-input-wrapper">
                <input
                  type="text"
                  readonly
                  :value="shareUrl"
                  class="copy-input"
                />
                <button class="copy-btn" :class="{ copied: copying }" @click="copyLink">
                  <svg v-if="!copying" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                    <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
                  </svg>
                  <svg v-else width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polyline points="20 6 9 17 4 12" />
                  </svg>
                  <span>{{ copying ? t('share.copied') : t('share.copy') }}</span>
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.modal-backdrop {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: var(--spacing-md);
}

.modal-container {
  background: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  width: 100%;
  max-width: 480px;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
}

.modal-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}

.close-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  background: transparent;
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.close-btn:hover {
  background: var(--color-bg-secondary);
  color: var(--color-text-primary);
}

.modal-body {
  padding: var(--spacing-lg);
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
}

/* Preview Card */
.preview-card {
  background: linear-gradient(135deg, #0f172a 0%, #1e1b4b 100%);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
  color: white;
}

.preview-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-md);
}

.preview-header svg {
  color: #FFD700;
}

.preview-badge {
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.1em;
  color: rgba(255, 255, 255, 0.7);
}

.preview-contest {
  font-size: var(--font-size-lg);
  font-weight: 700;
  margin: 0 0 var(--spacing-md) 0;
}

.preview-stats {
  display: flex;
  gap: var(--spacing-lg);
  flex-wrap: wrap;
}

.preview-stat {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.preview-stat .stat-label {
  font-size: var(--font-size-xs);
  color: rgba(255, 255, 255, 0.6);
  text-transform: uppercase;
}

.preview-stat .stat-value {
  font-size: var(--font-size-lg);
  font-weight: 700;
}

.preview-stat .stat-value.rank .of {
  font-size: var(--font-size-sm);
  font-weight: 400;
  opacity: 0.7;
  margin-left: 2px;
}

.preview-stat .stat-value.positive {
  color: #10B981;
}

.preview-stat .stat-value.negative {
  color: #EF4444;
}

.preview-stat .stat-value.prize {
  color: #FFD700;
}

/* Share Section */
.share-section {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.section-label {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-secondary);
}

.social-buttons {
  display: flex;
  gap: var(--spacing-sm);
}

.social-btn {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-sm) var(--spacing-md);
  border: none;
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: white;
  cursor: pointer;
  transition: all var(--transition-fast);
}

.social-btn.twitter {
  background: #000000;
}

.social-btn.twitter:hover {
  background: #14171a;
}

.social-btn.telegram {
  background: #0088cc;
}

.social-btn.telegram:hover {
  background: #006da8;
}

.social-btn.whatsapp {
  background: #25D366;
}

.social-btn.whatsapp:hover {
  background: #1da851;
}

/* Copy Section */
.copy-section {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.copy-input-wrapper {
  display: flex;
  gap: var(--spacing-sm);
}

.copy-input {
  flex: 1;
  padding: var(--spacing-sm) var(--spacing-md);
  background: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
  outline: none;
}

.copy-btn {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-sm) var(--spacing-md);
  background: var(--color-primary);
  border: none;
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: white;
  cursor: pointer;
  transition: all var(--transition-fast);
  white-space: nowrap;
}

.copy-btn:hover {
  background: var(--color-secondary);
}

.copy-btn.copied {
  background: #10B981;
}

/* Transitions */
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.2s ease;
}

.modal-enter-active .modal-container,
.modal-leave-active .modal-container {
  transition: transform 0.2s ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-from .modal-container,
.modal-leave-to .modal-container {
  transform: scale(0.95);
}

/* RTL Support */
[dir="rtl"] .preview-stat .stat-value.rank .of {
  margin-left: 0;
  margin-right: 2px;
}

/* Mobile */
@media (max-width: 767px) {
  .modal-container {
    max-height: 100vh;
    border-radius: var(--radius-lg) var(--radius-lg) 0 0;
    margin-top: auto;
  }

  .social-buttons {
    flex-direction: column;
  }

  .social-btn {
    justify-content: center;
  }

  .copy-input-wrapper {
    flex-direction: column;
  }
}
</style>
