<script setup lang="ts">
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { ticketsApi } from '../api/tickets';
import { useToast } from '@/composables/useToast';

const router = useRouter();
const toast = useToast();

const subject = ref('');
const category = ref('other');
const body = ref('');
const file = ref<File | null>(null);
const filePreview = ref<string | null>(null);
const submitting = ref(false);
const errors = ref<Record<string, string>>({});

const categories = [
  { value: 'account', label: () => t('tickets.category.account') },
  { value: 'payment', label: () => t('tickets.category.payment') },
  { value: 'contest', label: () => t('tickets.category.contest') },
  { value: 'technical', label: () => t('tickets.category.technical') },
  { value: 'kyc', label: () => t('tickets.category.kyc') },
  { value: 'other', label: () => t('tickets.category.other') },
];

const allowedTypes = ['image/jpeg', 'image/png', 'image/webp', 'application/pdf'];
const maxFileSize = 10 * 1024 * 1024;

function handleFileSelect(event: Event) {
  const input = event.target as HTMLInputElement;
  const selected = input.files?.[0];
  if (!selected) return;

  if (!allowedTypes.includes(selected.type)) {
    errors.value.file = t('tickets.invalidFileType');
    return;
  }
  if (selected.size > maxFileSize) {
    errors.value.file = t('tickets.fileTooLarge');
    return;
  }

  file.value = selected;
  errors.value.file = '';

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
  errors.value.file = '';
}

function validate(): boolean {
  errors.value = {};
  if (!subject.value.trim()) errors.value.subject = t('tickets.subject') + ' required';
  if (subject.value.length > 200) errors.value.subject = 'Max 200 characters';
  if (!body.value.trim()) errors.value.body = t('tickets.message') + ' required';
  if (body.value.length > 5000) errors.value.body = 'Max 5000 characters';
  return Object.keys(errors.value).length === 0;
}

async function handleSubmit() {
  if (!validate()) return;

  submitting.value = true;
  try {
    const formData = new FormData();
    formData.append('subject', subject.value.trim());
    formData.append('category', category.value);
    formData.append('body', body.value.trim());
    if (file.value) formData.append('attachment', file.value);

    const result = await ticketsApi.create(formData);
    toast.success(t('tickets.sent'));
    router.push(`/user/tickets/${result.id}`);
  } catch {
    // Error handled by interceptor
  } finally {
    submitting.value = false;
  }
}

function goBack() {
  router.push('/user/tickets');
}
</script>

<template>
  <div class="new-ticket-page">
    <div class="page-header">
      <button class="back-btn" @click="goBack">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="20" height="20"><polyline points="15 18 9 12 15 6" /></svg>
      </button>
      <h1>{{ t('tickets.newTicket') }}</h1>
    </div>

    <form class="ticket-form" @submit.prevent="handleSubmit">
      <div class="form-group">
        <label>{{ t('tickets.subject') }}</label>
        <input
          v-model="subject"
          type="text"
          :placeholder="t('tickets.subjectPlaceholder')"
          maxlength="200"
          class="input"
          :class="{ error: errors.subject }"
        />
        <span v-if="errors.subject" class="error-text">{{ errors.subject }}</span>
      </div>

      <div class="form-group">
        <label>{{ t('tickets.categoryLabel') }}</label>
        <select v-model="category" class="input">
          <option v-for="cat in categories" :key="cat.value" :value="cat.value">{{ cat.label() }}</option>
        </select>
      </div>

      <div class="form-group">
        <label>{{ t('tickets.message') }}</label>
        <textarea
          v-model="body"
          :placeholder="t('tickets.messagePlaceholder')"
          maxlength="5000"
          rows="6"
          class="input textarea"
          :class="{ error: errors.body }"
        />
        <div class="char-count">{{ body.length }}/5000</div>
        <span v-if="errors.body" class="error-text">{{ errors.body }}</span>
      </div>

      <div class="form-group">
        <label>{{ t('tickets.attachFile') }}</label>
        <p class="hint">{{ t('tickets.attachHint') }}</p>

        <div v-if="!file" class="file-drop" @click="($refs.fileInput as HTMLInputElement).click()">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="24" height="24"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><polyline points="17 8 12 3 7 8" /><line x1="12" y1="3" x2="12" y2="15" /></svg>
          <span>{{ t('tickets.attachFile') }}</span>
          <input ref="fileInput" type="file" accept=".jpg,.jpeg,.png,.webp,.pdf" style="display: none" @change="handleFileSelect" />
        </div>

        <div v-else class="file-preview">
          <img v-if="filePreview" :src="filePreview" class="preview-image" />
          <div v-else class="preview-pdf">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="24" height="24"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" /><polyline points="14 2 14 8 20 8" /></svg>
          </div>
          <div class="file-info">
            <span class="file-name">{{ file.name }}</span>
            <span class="file-size">{{ (file.size / 1024).toFixed(0) }} KB</span>
          </div>
          <button type="button" class="remove-file" @click="removeFile">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16"><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg>
          </button>
        </div>
        <span v-if="errors.file" class="error-text">{{ errors.file }}</span>
      </div>

      <button type="submit" class="btn-primary submit-btn" :disabled="submitting">
        {{ submitting ? t('tickets.sending') : t('tickets.send') }}
      </button>
    </form>
  </div>
</template>

<style scoped>
.new-ticket-page {
  max-width: 600px;
  margin: 0 auto;
  padding: 1.5rem 1rem;
}

.page-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 1.5rem;
}
.page-header h1 {
  font-size: 1.5rem;
  font-weight: 700;
  color: var(--theme-text, #fff);
  margin: 0;
}

.back-btn {
  background: none;
  border: none;
  color: var(--theme-text-secondary, #999);
  cursor: pointer;
  padding: 0.25rem;
  display: flex;
}

.ticket-form { display: flex; flex-direction: column; gap: 1.25rem; }

.form-group { display: flex; flex-direction: column; gap: 0.375rem; }
.form-group label {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--theme-text, #fff);
}

.input {
  padding: 0.75rem 1rem;
  background: var(--theme-glass, rgba(255,255,255,0.06));
  border: 1px solid rgba(255,255,255,0.1);
  border-radius: 0.75rem;
  color: var(--theme-text, #fff);
  font-size: 0.875rem;
  outline: none;
  transition: border-color 0.2s;
}
.input:focus { border-color: var(--theme-accent, #6366f1); }
.input.error { border-color: #ef4444; }

select.input {
  appearance: none;
  cursor: pointer;
}

.textarea { resize: vertical; min-height: 120px; font-family: inherit; }

.char-count {
  font-size: 0.75rem;
  color: var(--theme-text-secondary, #999);
  text-align: end;
}

.hint {
  font-size: 0.75rem;
  color: var(--theme-text-secondary, #999);
  margin: 0;
}

.error-text {
  font-size: 0.75rem;
  color: #ef4444;
}

.file-drop {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
  padding: 2rem;
  border: 2px dashed rgba(255,255,255,0.15);
  border-radius: 0.75rem;
  cursor: pointer;
  color: var(--theme-text-secondary, #999);
  transition: border-color 0.2s;
}
.file-drop:hover { border-color: var(--theme-accent, #6366f1); }

.file-preview {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.75rem;
  background: var(--theme-glass, rgba(255,255,255,0.06));
  border-radius: 0.75rem;
}
.preview-image {
  width: 48px;
  height: 48px;
  object-fit: cover;
  border-radius: 0.5rem;
}
.preview-pdf {
  width: 48px;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(239, 68, 68, 0.15);
  border-radius: 0.5rem;
  color: #ef4444;
}
.file-info { flex: 1; display: flex; flex-direction: column; }
.file-name { font-size: 0.8125rem; color: var(--theme-text, #fff); }
.file-size { font-size: 0.75rem; color: var(--theme-text-secondary, #999); }
.remove-file {
  background: none;
  border: none;
  color: var(--theme-text-secondary, #999);
  cursor: pointer;
  padding: 0.25rem;
}

.btn-primary {
  padding: 0.75rem 1.5rem;
  background: var(--theme-accent, #6366f1);
  color: #fff;
  border: none;
  border-radius: 0.75rem;
  font-size: 0.9375rem;
  font-weight: 600;
  cursor: pointer;
  transition: opacity 0.2s;
}
.btn-primary:hover { opacity: 0.9; }
.btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }

.submit-btn { align-self: stretch; }

/* RTL overrides */
[dir="rtl"] .back-btn svg {
  transform: scaleX(-1);
}
</style>
