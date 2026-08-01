<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { t } from '@/i18n';
import { adminTicketsApi, type AdminTicketDetail, type AdminTicketAttachment } from '@/api/tickets';
import { useToast } from '@/composables/useToast';
import { api } from '@/api';
import { useAuthStore } from '@/stores/auth';

const route = useRoute();
const router = useRouter();
const toast = useToast();

const ticketId = computed(() => route.params.id as string);
const detail = ref<AdminTicketDetail | null>(null);
const loading = ref(true);
const sending = ref(false);
const messageBody = ref('');
const file = ref<File | null>(null);
const filePreview = ref<string | null>(null);
const chatContainer = ref<HTMLDivElement | null>(null);

const ticket = computed(() => detail.value?.ticket);
const messages = computed(() => detail.value?.messages || []);
const isClosed = computed(() => ticket.value?.status === 'closed' || ticket.value?.status === 'resolved');

const authStore = useAuthStore();
const statusOptions = ['open', 'answered', 'user_replied', 'closed', 'resolved'];
const priorityOptions = ['low', 'medium', 'high', 'urgent'];

async function loadTicket() {
  loading.value = true;
  try {
    detail.value = await adminTicketsApi.get(ticketId.value);
    await nextTick();
    scrollToBottom();
  } catch {
    // handled by interceptor
  } finally {
    loading.value = false;
  }
}

function scrollToBottom() {
  if (chatContainer.value) {
    chatContainer.value.scrollTop = chatContainer.value.scrollHeight;
  }
}

function handleFileSelect(event: Event) {
  const input = event.target as HTMLInputElement;
  const selected = input.files?.[0];
  if (!selected) return;

  const allowedTypes = ['image/jpeg', 'image/png', 'image/webp', 'application/pdf'];
  if (!allowedTypes.includes(selected.type)) {
    toast.error(t('tickets.invalidFileType'));
    return;
  }
  if (selected.size > 10 * 1024 * 1024) {
    toast.error(t('tickets.fileTooLarge'));
    return;
  }

  file.value = selected;
  if (selected.type.startsWith('image/')) {
    const reader = new FileReader();
    reader.onload = (e) => { filePreview.value = e.target?.result as string; };
    reader.readAsDataURL(selected);
  } else {
    filePreview.value = null;
  }
}

function removeFile() {
  file.value = null;
  filePreview.value = null;
}

async function sendMessage() {
  if (!messageBody.value.trim() || sending.value) return;

  sending.value = true;
  try {
    const formData = new FormData();
    formData.append('body', messageBody.value.trim());
    if (file.value) formData.append('attachment', file.value);

    await adminTicketsApi.sendMessage(ticketId.value, formData);
    messageBody.value = '';
    file.value = null;
    filePreview.value = null;
    await loadTicket();
  } catch {
    // handled by interceptor
  } finally {
    sending.value = false;
  }
}

async function updateStatus(newStatus: string) {
  try {
    await adminTicketsApi.updateStatus(ticketId.value, newStatus);
    toast.success(t('tickets.statusUpdated'));
    await loadTicket();
  } catch {
    // handled by interceptor
  }
}

async function updatePriority(newPriority: string) {
  try {
    await adminTicketsApi.updatePriority(ticketId.value, newPriority);
    toast.success(t('tickets.priorityUpdated'));
    await loadTicket();
  } catch {
    // handled by interceptor
  }
}

async function assignToMe() {
  try {
    await adminTicketsApi.assign(ticketId.value, authStore.user?.id || '');
    toast.success(t('tickets.assigned'));
    await loadTicket();
  } catch {
    // handled by interceptor
  }
}

function goBack() {
  router.push('/admin/tickets');
}

function formatMessageTime(dateStr: string) {
  return new Date(dateStr).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function formatDate(dateStr: string) {
  return new Date(dateStr).toLocaleDateString(undefined, {
    year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  });
}

function shouldShowDateSeparator(index: number) {
  if (index === 0) return true;
  const curr = new Date(messages.value[index].created_at).toDateString();
  const prev = new Date(messages.value[index - 1].created_at).toDateString();
  return curr !== prev;
}

function formatDateSeparator(dateStr: string) {
  const date = new Date(dateStr);
  const today = new Date();
  if (date.toDateString() === today.toDateString()) return t('tickets.today');
  const yesterday = new Date(today);
  yesterday.setDate(yesterday.getDate() - 1);
  if (date.toDateString() === yesterday.toDateString()) return t('tickets.yesterday');
  return date.toLocaleDateString();
}

function isImageAttachment(att: AdminTicketAttachment) {
  return att.content_type.startsWith('image/');
}

const attachmentBlobUrls = ref<Map<string, string>>(new Map());

async function getSecureAttachmentUrl(attId: string): Promise<string> {
  const cached = attachmentBlobUrls.value.get(attId);
  if (cached) return cached;

  const url = adminTicketsApi.getAttachmentUrl(attId);
  const resp = await api.get(url, { responseType: 'blob' });
  const blobUrl = URL.createObjectURL(resp.data as Blob);
  attachmentBlobUrls.value.set(attId, blobUrl);
  return blobUrl;
}

function getAttachmentUrl(att: AdminTicketAttachment): string {
  // Return cached blob URL if available, otherwise trigger async fetch
  const cached = attachmentBlobUrls.value.get(att.id);
  if (cached) return cached;

  // Trigger async download — template will reactively update when blob URL is set
  getSecureAttachmentUrl(att.id);
  return '';
}

function formatFileSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function getStatusClass(status: string) {
  const map: Record<string, string> = {
    open: 'status-open', answered: 'status-answered',
    user_replied: 'status-waiting', closed: 'status-closed', resolved: 'status-resolved',
  };
  return map[status] || '';
}

// Poll for new messages every 10 seconds
let pollTimer: ReturnType<typeof setInterval> | null = null;

function startPolling() {
  if (pollTimer) clearInterval(pollTimer);
  pollTimer = setInterval(async () => {
    if (document.visibilityState !== 'visible') return;
    try {
      const prevCount = messages.value.length;
      detail.value = await adminTicketsApi.get(ticketId.value);
      if (messages.value.length > prevCount) {
        await nextTick();
        scrollToBottom();
      }
    } catch {
      // Silent fail
    }
  }, 10000);
}

onMounted(() => {
  loadTicket();
  startPolling();
});

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer);
  // Revoke blob URLs to free memory
  for (const blobUrl of attachmentBlobUrls.value.values()) {
    URL.revokeObjectURL(blobUrl);
  }
});
</script>

<template>
  <div class="ticket-detail-page">
    <!-- Header -->
    <div class="detail-header">
      <button class="back-btn" @click="goBack">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="20" height="20"><polyline points="15 18 9 12 15 6" /></svg>
      </button>
      <h1 v-if="ticket">{{ ticket.subject }}</h1>
      <span v-if="ticket" class="badge" :class="getStatusClass(ticket.status)">{{ t(`tickets.status.${ticket.status}`) }}</span>
    </div>

    <div v-if="loading" class="loading-spinner"><div class="spinner" /></div>

    <div v-else-if="ticket" class="detail-content">
      <!-- Chat area -->
      <div class="chat-area">
        <div ref="chatContainer" class="chat-messages">
          <template v-for="(msg, index) in messages" :key="msg.id">
            <div v-if="shouldShowDateSeparator(index)" class="date-separator">
              <span>{{ formatDateSeparator(msg.created_at) }}</span>
            </div>

            <div class="message" :class="{ own: msg.is_admin, other: !msg.is_admin }">
              <div class="message-bubble">
                <p class="message-body">{{ msg.body }}</p>
                <div v-if="msg.attachments.length > 0" class="attachments">
                  <template v-for="att in msg.attachments" :key="att.id">
                    <a v-if="isImageAttachment(att)" :href="getAttachmentUrl(att)" target="_blank" class="att-image">
                      <img :src="getAttachmentUrl(att)" :alt="att.file_name" />
                    </a>
                    <a v-else :href="getAttachmentUrl(att)" target="_blank" class="att-file">
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" /><polyline points="14 2 14 8 20 8" /></svg>
                      {{ att.file_name }} ({{ formatFileSize(att.file_size) }})
                    </a>
                  </template>
                </div>
                <span class="message-time">{{ formatMessageTime(msg.created_at) }}</span>
              </div>
              <span class="sender-label">{{ msg.sender_name }}</span>
            </div>
          </template>
        </div>

        <!-- Input -->
        <div v-if="isClosed" class="closed-notice">
          {{ t('tickets.closed') }}
        </div>
        <div v-else class="chat-input">
          <div v-if="file" class="file-preview-bar">
            <img v-if="filePreview" :src="filePreview" class="mini-img" />
            <div v-else class="mini-pdf">PDF</div>
            <span>{{ file.name }}</span>
            <button class="remove-file" @click="removeFile">x</button>
          </div>
          <div class="input-row">
            <button class="icon-btn" @click="($refs.fileInput as HTMLInputElement).click()">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="18" height="18"><path d="M21.44 11.05l-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48" /></svg>
              <input ref="fileInput" type="file" accept=".jpg,.jpeg,.png,.webp,.pdf" style="display:none" @change="handleFileSelect" />
            </button>
            <textarea
              v-model="messageBody"
              :placeholder="t('tickets.messagePlaceholder')"
              rows="1"
              class="msg-input"
              @keydown.enter.exact.prevent="sendMessage"
            />
            <button class="icon-btn send" :disabled="!messageBody.trim() || sending" @click="sendMessage">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="18" height="18"><line x1="22" y1="2" x2="11" y2="13" /><polygon points="22 2 15 22 11 13 2 9 22 2" /></svg>
            </button>
          </div>
        </div>
      </div>

      <!-- Info panel -->
      <div class="info-panel">
        <h3>{{ t('tickets.ticketInfo') }}</h3>

        <div class="info-group">
          <label>{{ t('tickets.categoryLabel') }}</label>
          <span class="badge category-badge">{{ t(`tickets.category.${ticket.category}`) }}</span>
        </div>

        <div class="info-group">
          <label>{{ t('tickets.userInfo') }}</label>
          <span>{{ ticket.user.username || ticket.user.email }}</span>
          <span class="sub-text">{{ ticket.user.email }}</span>
        </div>

        <div class="info-group">
          <label>{{ t('tickets.statusLabel') }}</label>
          <select :value="ticket.status" class="info-select" @change="updateStatus(($event.target as HTMLSelectElement).value)">
            <option v-for="s in statusOptions" :key="s" :value="s">{{ t(`tickets.status.${s}`) }}</option>
          </select>
        </div>

        <div class="info-group">
          <label>{{ t('tickets.priorityLabel') }}</label>
          <select :value="ticket.priority" class="info-select" @change="updatePriority(($event.target as HTMLSelectElement).value)">
            <option v-for="p in priorityOptions" :key="p" :value="p">{{ t(`tickets.priority.${p}`) }}</option>
          </select>
        </div>

        <div class="info-group">
          <label>{{ t('tickets.assignedTo') }}</label>
          <div v-if="ticket.assigned_admin" class="assigned-display">
            <span>{{ ticket.assigned_admin.username || ticket.assigned_admin.email }}</span>
          </div>
          <div v-else class="unassigned">{{ t('tickets.unassigned') }}</div>
          <button class="self-assign-btn" @click="assignToMe">
            {{ ticket.assigned_admin ? t('tickets.reassignToMe') : t('tickets.assignToMe') }}
          </button>
        </div>

        <div class="info-group">
          <label>{{ t('tickets.createdAt') }}</label>
          <span class="sub-text">{{ formatDate(ticket.created_at) }}</span>
        </div>

        <div class="info-group" v-if="ticket.closed_at">
          <label>{{ t('tickets.closedAt') }}</label>
          <span class="sub-text">{{ formatDate(ticket.closed_at) }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ticket-detail-page { padding: 1.5rem; height: 100%; display: flex; flex-direction: column; }

.detail-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 1rem;
}
.detail-header h1 {
  flex: 1;
  font-size: 1.25rem;
  font-weight: 700;
  color: var(--theme-text, #fff);
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.back-btn {
  background: none;
  border: none;
  color: var(--theme-text-secondary, #999);
  cursor: pointer;
  padding: 0.25rem;
  display: flex;
}

.badge {
  padding: 0.15rem 0.5rem;
  border-radius: 1rem;
  font-size: 0.6875rem;
  font-weight: 600;
}
.status-open { background: rgba(59, 130, 246, 0.15); color: #60a5fa; }
.status-answered { background: rgba(34, 197, 94, 0.15); color: #4ade80; }
.status-waiting { background: rgba(234, 179, 8, 0.15); color: #facc15; }
.status-closed { background: rgba(156, 163, 175, 0.15); color: #9ca3af; }
.status-resolved { background: rgba(34, 197, 94, 0.15); color: #4ade80; }
.category-badge { background: rgba(99, 102, 241, 0.15); color: var(--theme-accent, #6366f1); }

.loading-spinner { flex: 1; display: flex; align-items: center; justify-content: center; }
.spinner {
  width: 32px; height: 32px;
  border: 3px solid rgba(255,255,255,0.1);
  border-top-color: var(--theme-accent, #6366f1);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }

.detail-content {
  flex: 1;
  display: grid;
  grid-template-columns: 1fr 280px;
  gap: 1rem;
  min-height: 0;
}

.chat-area {
  display: flex;
  flex-direction: column;
  background: var(--theme-glass, rgba(255,255,255,0.03));
  border-radius: 0.75rem;
  overflow: hidden;
}

.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.date-separator { text-align: center; margin: 0.5rem 0; }
.date-separator span {
  font-size: 0.6875rem;
  color: var(--theme-text-secondary, #999);
  background: var(--theme-glass, rgba(255,255,255,0.06));
  padding: 0.2rem 0.6rem;
  border-radius: 1rem;
}

.message { display: flex; flex-direction: column; max-width: 75%; }
.message.own { align-self: flex-end; }
.message.other { align-self: flex-start; }

.message-bubble { padding: 0.625rem 0.875rem; border-radius: 0.875rem; }
.message.own .message-bubble {
  background: var(--theme-accent, #6366f1);
  color: #fff;
  border-bottom-right-radius: 0.2rem;
}
.message.other .message-bubble {
  background: var(--theme-glass, rgba(255,255,255,0.08));
  color: var(--theme-text, #fff);
  border-bottom-left-radius: 0.2rem;
}

.message-body { margin: 0; font-size: 0.8125rem; line-height: 1.4; white-space: pre-wrap; word-break: break-word; }
.message-time { display: block; font-size: 0.5625rem; opacity: 0.6; margin-top: 0.25rem; text-align: end; }
.sender-label { font-size: 0.625rem; color: var(--theme-text-secondary, #999); margin-top: 0.125rem; padding: 0 0.375rem; }
.message.own .sender-label { text-align: end; }

.attachments { margin-top: 0.375rem; display: flex; flex-direction: column; gap: 0.25rem; }
.att-image img { max-width: 180px; max-height: 180px; border-radius: 0.375rem; }
.att-file {
  display: flex; align-items: center; gap: 0.375rem;
  padding: 0.375rem; background: rgba(255,255,255,0.1);
  border-radius: 0.375rem; text-decoration: none; color: inherit;
  font-size: 0.75rem;
}

.chat-input {
  border-top: 1px solid rgba(255,255,255,0.08);
  padding: 0.625rem 0.75rem;
}

.file-preview-bar {
  display: flex; align-items: center; gap: 0.5rem;
  padding: 0.375rem; margin-bottom: 0.375rem;
  background: var(--theme-glass, rgba(255,255,255,0.06));
  border-radius: 0.375rem; font-size: 0.75rem; color: var(--theme-text, #fff);
}
.mini-img { width: 24px; height: 24px; object-fit: cover; border-radius: 0.2rem; }
.mini-pdf {
  width: 24px; height: 24px; display: flex; align-items: center; justify-content: center;
  background: rgba(239,68,68,0.15); color: #ef4444; font-size: 0.5rem; font-weight: 700; border-radius: 0.2rem;
}
.remove-file { background: none; border: none; color: var(--theme-text-secondary); cursor: pointer; }

.input-row { display: flex; align-items: flex-end; gap: 0.375rem; }
.icon-btn {
  background: none; border: none; color: var(--theme-text-secondary, #999);
  cursor: pointer; padding: 0.375rem; display: flex; transition: color 0.15s;
}
.icon-btn:hover { color: var(--theme-accent, #6366f1); }
.icon-btn.send:disabled { opacity: 0.3; cursor: not-allowed; }

.msg-input {
  flex: 1; padding: 0.5rem 0.625rem;
  background: var(--theme-glass, rgba(255,255,255,0.06));
  border: 1px solid rgba(255,255,255,0.1);
  border-radius: 0.625rem; color: var(--theme-text, #fff);
  font-size: 0.8125rem; font-family: inherit;
  resize: none; outline: none; min-height: 36px; max-height: 100px;
}
.msg-input:focus { border-color: var(--theme-accent, #6366f1); }

.closed-notice {
  text-align: center;
  padding: 1rem;
  color: var(--theme-text-secondary, #999);
  font-size: 0.875rem;
  border-top: 1px solid rgba(255,255,255,0.08);
}

/* Info panel */
.info-panel {
  background: var(--theme-glass, rgba(255,255,255,0.03));
  border-radius: 0.75rem;
  padding: 1.25rem;
  overflow-y: auto;
}
.info-panel h3 {
  font-size: 0.875rem;
  font-weight: 700;
  color: var(--theme-text, #fff);
  margin: 0 0 1rem;
}

.info-group {
  margin-bottom: 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}
.info-group label {
  font-size: 0.6875rem;
  font-weight: 600;
  color: var(--theme-text-secondary, #999);
  text-transform: uppercase;
}
.info-group span {
  font-size: 0.8125rem;
  color: var(--theme-text, #fff);
}
.sub-text {
  font-size: 0.75rem !important;
  color: var(--theme-text-secondary, #999) !important;
}

.self-assign-btn {
  margin-top: 0.25rem;
  padding: 0.3rem 0.6rem;
  font-size: 0.75rem;
  background: color-mix(in srgb, var(--theme-accent, #6366f1) 15%, transparent);
  color: var(--theme-accent, #6366f1);
  border: none;
  border-radius: 0.375rem;
  cursor: pointer;
  font-family: inherit;
}
.self-assign-btn:hover {
  background: color-mix(in srgb, var(--theme-accent, #6366f1) 25%, transparent);
}
.unassigned {
  color: var(--theme-text-secondary, #999);
  font-style: italic;
  font-size: 0.875rem;
}

.info-select {
  padding: 0.375rem 0.625rem;
  background: var(--theme-glass, rgba(255,255,255,0.06));
  border: 1px solid rgba(255,255,255,0.1);
  border-radius: 0.5rem;
  color: var(--theme-text, #fff);
  font-size: 0.8125rem;
  outline: none;
  appearance: none;
  cursor: pointer;
}

@media (max-width: 768px) {
  .detail-content { grid-template-columns: 1fr; }
  .info-panel { order: -1; }
}

/* RTL overrides */
[dir="rtl"] .back-btn svg {
  transform: scaleX(-1);
}
[dir="rtl"] .message.own .message-bubble {
  border-bottom-right-radius: 1rem;
  border-bottom-left-radius: 0.25rem;
}
[dir="rtl"] .message.other .message-bubble {
  border-bottom-left-radius: 1rem;
  border-bottom-right-radius: 0.25rem;
}
[dir="rtl"] .icon-btn.send svg {
  transform: scaleX(-1);
}
</style>
