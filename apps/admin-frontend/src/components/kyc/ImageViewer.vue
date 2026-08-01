<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { t } from '@/i18n';

interface Props {
  show: boolean;
  imageUrl: string;
  title?: string;
}

const props = defineProps<Props>();
const emit = defineEmits<{
  close: [];
}>();

const scale = ref(1);
const position = ref({ x: 0, y: 0 });
const isDragging = ref(false);
const dragStart = ref({ x: 0, y: 0 });

const MIN_SCALE = 0.5;
const MAX_SCALE = 4;
const SCALE_STEP = 0.25;

const imageStyle = computed(() => ({
  transform: `translate(${position.value.x}px, ${position.value.y}px) scale(${scale.value})`,
}));

function zoomIn(): void {
  scale.value = Math.min(MAX_SCALE, scale.value + SCALE_STEP);
}

function zoomOut(): void {
  scale.value = Math.max(MIN_SCALE, scale.value - SCALE_STEP);
}

function resetZoom(): void {
  scale.value = 1;
  position.value = { x: 0, y: 0 };
}

function handleWheel(event: WheelEvent): void {
  event.preventDefault();
  if (event.deltaY < 0) {
    zoomIn();
  } else {
    zoomOut();
  }
}

function startDrag(event: MouseEvent): void {
  isDragging.value = true;
  dragStart.value = {
    x: event.clientX - position.value.x,
    y: event.clientY - position.value.y,
  };
}

function onDrag(event: MouseEvent): void {
  if (!isDragging.value) return;
  position.value = {
    x: event.clientX - dragStart.value.x,
    y: event.clientY - dragStart.value.y,
  };
}

function endDrag(): void {
  isDragging.value = false;
}

function downloadImage(): void {
  const link = document.createElement('a');
  link.href = props.imageUrl;
  link.download = props.title || 'document';
  link.target = '_blank';
  link.click();
}

function handleClose(): void {
  emit('close');
}

function handleBackdropClick(event: MouseEvent): void {
  if ((event.target as HTMLElement).classList.contains('image-viewer-overlay')) {
    handleClose();
  }
}

function handleKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') {
    handleClose();
  } else if (event.key === '+' || event.key === '=') {
    zoomIn();
  } else if (event.key === '-') {
    zoomOut();
  } else if (event.key === '0') {
    resetZoom();
  }
}

watch(() => props.show, (newVal) => {
  if (newVal) {
    resetZoom();
    document.addEventListener('keydown', handleKeydown);
  } else {
    document.removeEventListener('keydown', handleKeydown);
  }
});
</script>

<template>
  <Teleport to="body">
    <div
      v-if="show"
      class="image-viewer-overlay"
      @click="handleBackdropClick"
      @mouseup="endDrag"
      @mousemove="onDrag"
    >
      <div class="image-viewer-header">
        <span class="image-title">{{ title }}</span>
        <div class="image-controls">
          <button class="control-btn" @click="zoomOut" :title="t('kyc.zoomOut')">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="11" cy="11" r="8" />
              <line x1="21" y1="21" x2="16.65" y2="16.65" />
              <line x1="8" y1="11" x2="14" y2="11" />
            </svg>
          </button>
          <span class="zoom-level">{{ Math.round(scale * 100) }}%</span>
          <button class="control-btn" @click="zoomIn" :title="t('kyc.zoomIn')">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="11" cy="11" r="8" />
              <line x1="21" y1="21" x2="16.65" y2="16.65" />
              <line x1="11" y1="8" x2="11" y2="14" />
              <line x1="8" y1="11" x2="14" y2="11" />
            </svg>
          </button>
          <button class="control-btn" @click="resetZoom" :title="t('kyc.resetZoom')">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
              <path d="M3 3v5h5" />
            </svg>
          </button>
          <div class="control-divider"></div>
          <button class="control-btn" @click="downloadImage" :title="t('kyc.downloadImage')">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
              <polyline points="7 10 12 15 17 10" />
              <line x1="12" y1="15" x2="12" y2="3" />
            </svg>
          </button>
          <button class="control-btn close-btn" @click="handleClose" :title="t('common.close')">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>
      </div>
      <div
        class="image-container"
        @wheel="handleWheel"
        @mousedown="startDrag"
      >
        <img
          :src="imageUrl"
          :alt="title"
          :style="imageStyle"
          class="viewer-image"
          :class="{ dragging: isDragging }"
          draggable="false"
        />
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.image-viewer-overlay {
  position: fixed;
  inset: 0;
  background-color: rgba(0, 0, 0, 0.9);
  z-index: var(--z-modal);
  display: flex;
  flex-direction: column;
}

.image-viewer-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--spacing-md) var(--spacing-lg);
  background-color: rgba(0, 0, 0, 0.5);
}

.image-title {
  color: white;
  font-size: var(--font-size-lg);
  font-weight: 500;
}

.image-controls {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.control-btn {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: rgba(255, 255, 255, 0.1);
  border: none;
  border-radius: var(--radius-md);
  color: white;
  cursor: pointer;
  transition: background-color var(--transition-fast);
}

.control-btn:hover {
  background-color: rgba(255, 255, 255, 0.2);
}

.control-btn svg {
  width: 20px;
  height: 20px;
}

.close-btn:hover {
  background-color: rgba(239, 68, 68, 0.5);
}

.zoom-level {
  color: white;
  font-size: var(--font-size-sm);
  min-width: 50px;
  text-align: center;
  font-family: var(--font-family-mono);
}

.control-divider {
  width: 1px;
  height: 24px;
  background-color: rgba(255, 255, 255, 0.2);
  margin: 0 var(--spacing-xs);
}

.image-container {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  cursor: grab;
}

.viewer-image {
  max-width: 90%;
  max-height: 90%;
  object-fit: contain;
  transition: transform 0.1s ease-out;
  user-select: none;
}

.viewer-image.dragging {
  cursor: grabbing;
  transition: none;
}
</style>
