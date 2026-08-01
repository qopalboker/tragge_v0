<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { t } from '@/i18n';
import { ticketsApi, type TicketDetail, type TicketMessage, type TicketAttachment } from '../api/tickets';
import { useToast } from '@/composables/useToast';
import { api } from '../api';

const route = useRoute();
const router = useRouter();
const toast = useToast();

const ticketId = computed(() => route.params.ticketId as string);
const ticketDetail = ref<TicketDetail | null>(null);
const loading = ref(true);
const sending = ref(false);
const messageBody = ref('');
const file = ref<File | null>(null);
const filePreview = ref<string | null>(null);
const chatContainer = ref<HTMLDivElement | null>(null);

const allowedTypes = ['image/jpeg', 'image/png', 'image/webp', 'application/pdf'];
const maxFileSize = 10 * 1024 * 1024;

const ticket = computed(() => ticketDetail.value?.ticket);
const messages = computed(() => ticketDetail.value?.messages || []);
const isClosed = computed(() => ticket.value?.status === 'closed' || ticket.value?.status === 'resolved');

async function loadTicket() {
  loading.value = true;
  try {
    ticketDetail.value = await ticketsApi.get(ticketId.value);
    await nextTick();
    scrollToBottom();
  } catch {
    // Error handled by interceptor
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

  if (!allowedTypes.includes(selected.type)) {
    toast.error(t('tickets.invalidFileType'));
    return;
  }
  if (selected.size > maxFileSize) {
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

    await ticketsApi.sendMessage(ticketId.value, formData);
    messageBody.value = '';
    file.value = null;
    filePreview.value = null;
    await loadTicket();
  } catch {
    // Error handled by interceptor
  } finally {
    sending.value = false;
  }
}

async function closeTicket() {
  try {
    await ticketsApi.close(ticketId.value);
    toast.success(t('tickets.closed'));
    await loadTicket();
  } catch {
    // Error handled by interceptor
  }
}

function goBack() {
  router.push('/user/tickets');
}

function getStatusLabel(status: string) {
  return t(`tickets.status.${status}`) || status;
}

function getStatusClass(status: string) {
  const map: Record<string, string> = {
    open: 'status-open', answered: 'status-answered',
    user_replied: 'status-waiting', closed: 'status-closed', resolved: 'status-resolved',
  };
  return map[status] || '';
}

function formatMessageTime(dateStr: string) {
  const date = new Date(dateStr);
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function formatDateSeparator(dateStr: string) {
  const date = new Date(dateStr);
  const today = new Date();
  const yesterday = new Date(today);
  yesterday.setDate(yesterday.getDate() - 1);

  if (date.toDateString() === today.toDateString()) return t('tickets.today');
  if (date.toDateString() === yesterday.toDateString()) return t('tickets.yesterday');
  return date.toLocaleDateString();
}

function shouldShowDateSeparator(index: number) {
  if (index === 0) return true;
  const curr = new Date(messages.value[index].created_at).toDateString();
  const prev = new Date(messages.value[index - 1].created_at).toDateString();
  return curr !== prev;
}

function isImageAttachment(att: TicketAttachment) {
  return att.content_type.startsWith('image/');
}

const attachmentBlobUrls = ref<Map<string, string>>(new Map());

async function getSecureAttachmentUrl(attId: string): Promise<string> {
  const cached = attachmentBlobUrls.value.get(attId);
  if (cached) return cached;

  const url = ticketsApi.getAttachmentUrl(attId);
  const resp = await api.get(url, { responseType: 'blob' });
  const blobUrl = URL.createObjectURL(resp.data as Blob);
  attachmentBlobUrls.value.set(attId, blobUrl);
  return blobUrl;
}

function getAttachmentUrl(att: TicketAttachment): string {
  const cached = attachmentBlobUrls.value.get(att.id);
  if (cached) return cached;

  getSecureAttachmentUrl(att.id);
  return '';
}

function formatFileSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

// Poll for new messages every 10 seconds
let pollTimer: ReturnType<typeof setInterval> | null = null;

function startPolling() {
  stopPolling();
  pollTimer = setInterval(async () => {
    if (document.visibilityState !== 'visible' || isClosed.value) return;
    try {
      const prevCount = messages.value.length;
      ticketDetail.value = await ticketsApi.get(ticketId.value);
      if (messages.value.length > prevCount) {
        await nextTick();
        scrollToBottom();
      }
    } catch {
      // Silent fail — next poll will retry
    }
  }, 10000);
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

onMounted(() => {
  loadTicket();
  startPolling();
});

onUnmounted(() => {
  stopPolling();
  for (const blobUrl of attachmentBlobUrls.value.values()) {
    URL.revokeObjectURL(blobUrl);
  }
});
</script>

<template>
  <div class="chat-page">
    <!-- Header -->
    <div class="chat-header">
      <button class="back-btn" @click="goBack">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="20" height="20"><polyline points="15 18 9 12 15 6" /></svg>
      </button>
      <div class="header-info" v-if="ticket">
        <h2>{{ ticket.subject }}</h2>
        <span class="badge" :class="getStatusClass(ticket.status)">{{ getStatusLabel(ticket.status) }}</span>
      </div>
      <button v-if="ticket && !isClosed" class="close-ticket-btn" @click="closeTicket">
        {{ t('tickets.close') }}
      </button>
    </div>

    <!-- Chat area -->
    <div v-if="loading" class="chat-loading">
      <div class="spinner" />
    </div>

    <div v-else ref="chatContainer" class="chat-container">
      <template v-for="(msg, index) in messages" :key="msg.id">
        <div v-if="shouldShowDateSeparator(index)" class="date-separator">
          <span>{{ formatDateSeparator(msg.created_at) }}</span>
        </div>

        <div class="message" :class="{ own: !msg.is_admin, admin: msg.is_admin }">
          <div class="message-bubble">
            <p class="message-body">{{ msg.body }}</p>

            <!-- Attachments -->
            <div v-if="msg.attachments.length > 0" class="attachments">
              <template v-for="att in msg.attachments" :key="att.id">
                <a v-if="isImageAttachment(att)" :href="getAttachmentUrl(att)" target="_blank" class="attachment-image">
                  <img :src="getAttachmentUrl(att)" :alt="att.file_name" />
                </a>
                <a v-else :href="getAttachmentUrl(att)" target="_blank" class="attachment-file">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="18" height="18"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" /><polyline points="14 2 14 8 20 8" /></svg>
                  <span class="att-name">{{ att.file_name }}</span>
                  <span class="att-size">{{ formatFileSize(att.file_size) }}</span>
                </a>
              </template>
            </div>

            <span class="message-time">{{ formatMessageTime(msg.created_at) }}</span>
          </div>
          <span class="sender-name">{{ msg.sender_name }}</span>
        </div>
      </template>
    </div>

    <!-- Input area -->
    <div v-if="isClosed" class="closed-notice">
      {{ t('tickets.closed') }}
    </div>

    <div v-else class="chat-input">
      <div v-if="file" class="input-file-preview">
        <img v-if="filePreview" :src="filePreview" class="mini-preview" />
        <div v-else class="mini-pdf">PDF</div>
        <span class="mini-name">{{ file.name }}</span>
        <button class="remove-file" @click="removeFile">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="14" height="14"><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg>
        </button>
      </div>

      <div class="input-row">
        <button class="attach-btn" @click="($refs.fileInput as HTMLInputElement).click()">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="20" height="20"><path d="M21.44 11.05l-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48" /></svg>
          <input ref="fileInput" type="file" accept=".jpg,.jpeg,.png,.webp,.pdf" style="display: none" @change="handleFileSelect" />
        </button>

        <textarea
          v-model="messageBody"
          :placeholder="t('tickets.messagePlaceholder')"
          rows="1"
          class="message-input"
          @keydown.enter.exact.prevent="sendMessage"
        />

        <button class="send-btn" :disabled="!messageBody.trim() || sending" @click="sendMessage">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="20" height="20"><line x1="22" y1="2" x2="11" y2="13" /><polygon points="22 2 15 22 11 13 2 9 22 2" /></svg>
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.chat-page {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 64px);
  max-width: 700px;
  margin: -32px auto;
  padding: 0;
}

@media (max-width: 767px) {
  .chat-page {
    height: calc(100dvh - var(--bottom-nav-height, 64px));
    margin: -12px -10px;
  }
}

.chat-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 1rem;
  border-bottom: 1px solid rgba(255,255,255,0.08);
}

.back-btn {
  background: none;
  border: none;
  color: var(--theme-text-secondary, #999);
  cursor: pointer;
  padding: 0.25rem;
  display: flex;
}

.header-info {
  flex: 1;
  min-width: 0;
}
.header-info h2 {
  font-size: 1rem;
  font-weight: 600;
  color: var(--theme-text, #fff);
  margin: 0 0 0.25rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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

.close-ticket-btn {
  padding: 0.4rem 0.8rem;
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
  border: none;
  border-radius: 0.5rem;
  font-size: 0.75rem;
  font-weight: 600;
  cursor: pointer;
}

.chat-loading {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}
.spinner {
  width: 32px;
  height: 32px;
  border: 3px solid rgba(255,255,255,0.1);
  border-top-color: var(--theme-accent, #6366f1);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }

.chat-container {
  flex: 1;
  overflow-y: auto;
  padding: 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.date-separator {
  text-align: center;
  margin: 0.5rem 0;
}
.date-separator span {
  font-size: 0.75rem;
  color: var(--theme-text-secondary, #999);
  background: var(--theme-glass, rgba(255,255,255,0.06));
  padding: 0.25rem 0.75rem;
  border-radius: 1rem;
}

.message {
  display: flex;
  flex-direction: column;
  max-width: 80%;
  animation: fadeIn 0.3s ease;
}
@keyframes fadeIn {
  from { opacity: 0; transform: translateY(8px); }
  to { opacity: 1; transform: translateY(0); }
}

.message.own {
  align-self: flex-end;
}
.message.admin {
  align-self: flex-start;
}

.message-bubble {
  padding: 0.75rem 1rem;
  border-radius: 1rem;
  position: relative;
}
.message.own .message-bubble {
  background: var(--theme-accent, #6366f1);
  color: #fff;
  border-bottom-right-radius: 0.25rem;
}
.message.admin .message-bubble {
  background: var(--theme-glass, rgba(255,255,255,0.08));
  color: var(--theme-text, #fff);
  border-bottom-left-radius: 0.25rem;
}

.message-body {
  margin: 0;
  font-size: 0.875rem;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}

.message-time {
  display: block;
  font-size: 0.625rem;
  opacity: 0.6;
  margin-top: 0.375rem;
  text-align: end;
}

.sender-name {
  font-size: 0.6875rem;
  color: var(--theme-text-secondary, #999);
  margin-top: 0.25rem;
  padding: 0 0.5rem;
}
.message.own .sender-name { text-align: end; }

.attachments {
  margin-top: 0.5rem;
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
}

.attachment-image {
  display: block;
}
.attachment-image img {
  max-width: 200px;
  max-height: 200px;
  border-radius: 0.5rem;
  cursor: pointer;
}

.attachment-file {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem;
  background: rgba(255,255,255,0.1);
  border-radius: 0.5rem;
  text-decoration: none;
  color: inherit;
}
.att-name { font-size: 0.8125rem; }
.att-size { font-size: 0.6875rem; opacity: 0.6; }

.closed-notice {
  text-align: center;
  padding: 1rem;
  color: var(--theme-text-secondary, #999);
  font-size: 0.875rem;
  border-top: 1px solid rgba(255,255,255,0.08);
}

.chat-input {
  border-top: 1px solid rgba(255,255,255,0.08);
  padding: 0.75rem 1rem;
}

.input-file-preview {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem;
  margin-bottom: 0.5rem;
  background: var(--theme-glass, rgba(255,255,255,0.06));
  border-radius: 0.5rem;
}
.mini-preview {
  width: 32px;
  height: 32px;
  object-fit: cover;
  border-radius: 0.25rem;
}
.mini-pdf {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
  font-size: 0.625rem;
  font-weight: 700;
  border-radius: 0.25rem;
}
.mini-name {
  flex: 1;
  font-size: 0.75rem;
  color: var(--theme-text, #fff);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.remove-file {
  background: none;
  border: none;
  color: var(--theme-text-secondary, #999);
  cursor: pointer;
  padding: 0.25rem;
}

.input-row {
  display: flex;
  align-items: flex-end;
  gap: 0.5rem;
}

.attach-btn, .send-btn {
  background: none;
  border: none;
  color: var(--theme-text-secondary, #999);
  cursor: pointer;
  padding: 0.5rem;
  display: flex;
  transition: color 0.2s;
}
.attach-btn:hover, .send-btn:hover:not(:disabled) { color: var(--theme-accent, #6366f1); }
.send-btn:disabled { opacity: 0.3; cursor: not-allowed; }

.message-input {
  flex: 1;
  padding: 0.625rem 0.75rem;
  background: var(--theme-glass, rgba(255,255,255,0.06));
  border: 1px solid rgba(255,255,255,0.1);
  border-radius: 0.75rem;
  color: var(--theme-text, #fff);
  font-size: 0.875rem;
  font-family: inherit;
  resize: none;
  outline: none;
  min-height: 40px;
  max-height: 120px;
}
.message-input:focus { border-color: var(--theme-accent, #6366f1); }

/* RTL overrides */
[dir="rtl"] .back-btn svg {
  transform: scaleX(-1);
}
[dir="rtl"] .message.own .message-bubble {
  border-bottom-right-radius: 1rem;
  border-bottom-left-radius: 0.25rem;
}
[dir="rtl"] .message.admin .message-bubble {
  border-bottom-left-radius: 1rem;
  border-bottom-right-radius: 0.25rem;
}
[dir="rtl"] .send-btn svg {
  transform: scaleX(-1);
}
</style>
