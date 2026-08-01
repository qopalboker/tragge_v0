<script setup lang="ts">
import { ref, computed } from 'vue';
import { t } from '@/i18n';

interface Props {
  label: string;
  accept?: string;
  maxSizeMB?: number;
  required?: boolean;
  showCamera?: boolean;
  hint?: string;
}

const props = withDefaults(defineProps<Props>(), {
  accept: 'image/jpeg,image/png,image/webp',
  maxSizeMB: 10,
  required: false,
  showCamera: false,
  hint: '',
});

const emit = defineEmits<{
  (e: 'update:file', file: File | null): void;
  (e: 'error', message: string): void;
}>();

const isDragging = ref(false);
const previewUrl = ref<string | null>(null);
const fileName = ref<string | null>(null);
const fileInputRef = ref<HTMLInputElement | null>(null);
const cameraStream = ref<MediaStream | null>(null);
const videoRef = ref<HTMLVideoElement | null>(null);
const showCameraModal = ref(false);

const maxSizeBytes = computed(() => props.maxSizeMB * 1024 * 1024);

const acceptedExtensions = computed(() => {
  return props.accept
    .split(',')
    .map((type) => type.trim().replace('image/', '.').toUpperCase())
    .join(', ');
});

function validateFile(file: File): boolean {
  // Check type
  const validTypes = props.accept.split(',').map((t) => t.trim());
  if (!validTypes.includes(file.type)) {
    emit('error', t('kyc.invalidFileType', { types: acceptedExtensions.value }));
    return false;
  }

  // Check size
  if (file.size > maxSizeBytes.value) {
    emit('error', t('kyc.fileTooLarge', { size: props.maxSizeMB }));
    return false;
  }

  return true;
}

function handleFile(file: File): void {
  if (!validateFile(file)) {
    return;
  }

  // Create preview
  const reader = new FileReader();
  reader.onload = (e) => {
    previewUrl.value = e.target?.result as string;
    fileName.value = file.name;
    emit('update:file', file);
  };
  reader.readAsDataURL(file);
}

function handleDragOver(e: DragEvent): void {
  e.preventDefault();
  isDragging.value = true;
}

function handleDragLeave(e: DragEvent): void {
  e.preventDefault();
  isDragging.value = false;
}

function handleDrop(e: DragEvent): void {
  e.preventDefault();
  isDragging.value = false;

  const files = e.dataTransfer?.files;
  if (files && files.length > 0) {
    handleFile(files[0]);
  }
}

function handleInputChange(e: Event): void {
  const input = e.target as HTMLInputElement;
  const files = input.files;
  if (files && files.length > 0) {
    handleFile(files[0]);
  }
}

function triggerFileInput(): void {
  fileInputRef.value?.click();
}

function removeFile(): void {
  previewUrl.value = null;
  fileName.value = null;
  if (fileInputRef.value) {
    fileInputRef.value.value = '';
  }
  emit('update:file', null);
}

async function openCamera(): Promise<void> {
  try {
    const stream = await navigator.mediaDevices.getUserMedia({
      video: { facingMode: 'user', width: { ideal: 1280 }, height: { ideal: 720 } },
    });
    cameraStream.value = stream;
    showCameraModal.value = true;

    // Wait for modal to render
    setTimeout(() => {
      if (videoRef.value && cameraStream.value) {
        videoRef.value.srcObject = cameraStream.value;
        videoRef.value.play();
      }
    }, 100);
  } catch {
    emit('error', t('kyc.cameraAccessDenied'));
  }
}

function capturePhoto(): void {
  if (!videoRef.value) return;

  const canvas = document.createElement('canvas');
  canvas.width = videoRef.value.videoWidth;
  canvas.height = videoRef.value.videoHeight;
  const ctx = canvas.getContext('2d');

  if (ctx) {
    ctx.drawImage(videoRef.value, 0, 0);
    canvas.toBlob(
      (blob) => {
        if (blob) {
          const file = new File([blob], 'selfie.jpg', { type: 'image/jpeg' });
          handleFile(file);
          closeCamera();
        }
      },
      'image/jpeg',
      0.9
    );
  }
}

function closeCamera(): void {
  if (cameraStream.value) {
    cameraStream.value.getTracks().forEach((track) => track.stop());
    cameraStream.value = null;
  }
  showCameraModal.value = false;
}
</script>

<template>
  <div class="file-upload">
    <label v-if="label" class="file-upload-label">
      {{ label }}
      <span v-if="required" class="required">*</span>
    </label>

    <!-- Preview State -->
    <div v-if="previewUrl" class="file-preview">
      <img :src="previewUrl" :alt="fileName || 'Preview'" class="preview-image" />
      <div class="preview-overlay">
        <button type="button" class="preview-action remove" @click="removeFile">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="18" y1="6" x2="6" y2="18" />
            <line x1="6" y1="6" x2="18" y2="18" />
          </svg>
        </button>
        <button type="button" class="preview-action replace" @click="triggerFileInput">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M23 4v6h-6" />
            <path d="M1 20v-6h6" />
            <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
          </svg>
        </button>
      </div>
      <span class="preview-filename">{{ fileName }}</span>
    </div>

    <!-- Upload Zone -->
    <div
      v-else
      :class="['drop-zone', { 'is-dragging': isDragging }]"
      @dragover="handleDragOver"
      @dragleave="handleDragLeave"
      @drop="handleDrop"
      @click="triggerFileInput"
    >
      <input
        ref="fileInputRef"
        type="file"
        :accept="accept"
        class="hidden-input"
        @change="handleInputChange"
      />

      <div class="drop-zone-content">
        <svg class="upload-icon" width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
          <polyline points="17 8 12 3 7 8" />
          <line x1="12" y1="3" x2="12" y2="15" />
        </svg>
        <p class="drop-zone-text">{{ t('kyc.dropFileHere') }}</p>
        <p class="drop-zone-subtext">{{ t('kyc.orClickToBrowse') }}</p>

        <div v-if="showCamera" class="camera-option">
          <span class="divider-text">{{ t('kyc.or') }}</span>
          <button type="button" class="camera-btn" @click.stop="openCamera">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z" />
              <circle cx="12" cy="13" r="4" />
            </svg>
            {{ t('kyc.takePhoto') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Hint -->
    <p v-if="hint" class="file-hint">{{ hint }}</p>
    <p class="file-requirements">
      {{ t('kyc.acceptedFormats') }}: {{ acceptedExtensions }} ({{ t('kyc.maxSize') }}: {{ maxSizeMB }}MB)
    </p>

    <!-- Camera Modal -->
    <Teleport to="body">
      <div v-if="showCameraModal" class="camera-modal">
        <div class="camera-modal-backdrop" @click="closeCamera" />
        <div class="camera-modal-content">
          <div class="camera-header">
            <h3>{{ t('kyc.takePhoto') }}</h3>
            <button type="button" class="close-btn" @click="closeCamera">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="18" y1="6" x2="6" y2="18" />
                <line x1="6" y1="6" x2="18" y2="18" />
              </svg>
            </button>
          </div>
          <div class="camera-viewport">
            <video ref="videoRef" autoplay playsinline class="camera-video" />
            <div class="camera-guide" />
          </div>
          <div class="camera-actions">
            <button type="button" class="btn btn-secondary" @click="closeCamera">
              {{ t('common.cancel') }}
            </button>
            <button type="button" class="btn btn-primary capture-btn" @click="capturePhoto">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10" />
                <circle cx="12" cy="12" r="6" fill="currentColor" />
              </svg>
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.file-upload {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.file-upload-label {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
}

.required {
  color: var(--color-error);
  margin-left: var(--spacing-xs);
}

[dir="rtl"] .required {
  margin-left: 0;
  margin-right: var(--spacing-xs);
}

.hidden-input {
  display: none;
}

.drop-zone {
  border: 2px dashed var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--spacing-xl);
  text-align: center;
  cursor: pointer;
  transition: all var(--transition-fast);
  background-color: var(--color-bg-secondary);
}

.drop-zone:hover,
.drop-zone.is-dragging {
  border-color: var(--color-primary);
  background-color: var(--color-primary-light);
}

.drop-zone-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-sm);
}

.upload-icon {
  color: var(--color-text-muted);
  margin-bottom: var(--spacing-sm);
}

.drop-zone:hover .upload-icon,
.drop-zone.is-dragging .upload-icon {
  color: var(--color-primary);
}

.drop-zone-text {
  font-weight: 500;
  color: var(--color-text-primary);
  margin: 0;
}

.drop-zone-subtext {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin: 0;
}

.camera-option {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-sm);
  margin-top: var(--spacing-md);
}

.divider-text {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
}

.camera-btn {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
  cursor: pointer;
  transition: all var(--transition-fast);
}

.camera-btn:hover {
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.file-preview {
  position: relative;
  border-radius: var(--radius-lg);
  overflow: hidden;
  background-color: var(--color-bg-secondary);
}

.preview-image {
  width: 100%;
  height: 200px;
  object-fit: cover;
}

.preview-overlay {
  position: absolute;
  top: var(--spacing-sm);
  right: var(--spacing-sm);
  display: flex;
  gap: var(--spacing-xs);
}

[dir="rtl"] .preview-overlay {
  right: auto;
  left: var(--spacing-sm);
}

.preview-action {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: rgba(0, 0, 0, 0.6);
  border: none;
  border-radius: var(--radius-full);
  color: white;
  cursor: pointer;
  transition: background-color var(--transition-fast);
}

.preview-action:hover {
  background-color: rgba(0, 0, 0, 0.8);
}

.preview-action.remove:hover {
  background-color: var(--color-error);
}

.preview-filename {
  display: block;
  padding: var(--spacing-sm) var(--spacing-md);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  text-align: center;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.file-hint {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin: 0;
}

.file-requirements {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  margin: 0;
}

/* Camera Modal */
.camera-modal {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
}

.camera-modal-backdrop {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: rgba(0, 0, 0, 0.8);
}

.camera-modal-content {
  position: relative;
  width: 100%;
  max-width: 480px;
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-xl);
  overflow: hidden;
  margin: var(--spacing-md);
}

.camera-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--spacing-md) var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
}

.camera-header h3 {
  margin: 0;
  font-size: var(--font-size-md);
  font-weight: 600;
}

.close-btn {
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: none;
  border-radius: var(--radius-full);
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: background-color var(--transition-fast);
}

.close-btn:hover {
  background-color: var(--color-bg-secondary);
  color: var(--color-text-primary);
}

.camera-viewport {
  position: relative;
  background-color: black;
}

.camera-video {
  width: 100%;
  display: block;
  transform: scaleX(-1);
}

.camera-guide {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: 200px;
  height: 250px;
  border: 3px solid rgba(255, 255, 255, 0.5);
  border-radius: var(--radius-xl);
  pointer-events: none;
}

.camera-actions {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: var(--spacing-lg);
  padding: var(--spacing-lg);
  background-color: var(--color-bg-secondary);
}

.capture-btn {
  width: 64px;
  height: 64px;
  border-radius: var(--radius-full);
  padding: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

@media (max-width: 767px) {
  .drop-zone {
    padding: var(--spacing-lg);
  }

  .upload-icon {
    width: 40px;
    height: 40px;
  }

  .preview-image {
    height: 160px;
  }

  .camera-modal-content {
    max-height: 90vh;
  }

  .camera-guide {
    width: 160px;
    height: 200px;
  }
}
</style>
