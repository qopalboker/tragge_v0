<script setup lang="ts">
import { ref, watch, onMounted } from 'vue';
import { t } from '@/i18n';
import { profileApi, type PredefinedAvatar } from '@/modules/user/api';

const props = defineProps<{
  show: boolean;
  currentAvatarId?: string;
}>();

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void;
  (e: 'select', avatar: PredefinedAvatar): void;
}>();

const selectedAvatarId = ref<string | null>(null);
const avatars = ref<PredefinedAvatar[]>([]);
const loading = ref(true);

onMounted(async () => {
  try {
    const response = await profileApi.listAvatars();
    avatars.value = response.avatars || [];
  } catch (e) {
    console.error('Failed to load avatars', e);
  } finally {
    loading.value = false;
  }
});

watch(
  () => props.show,
  (newVal) => {
    if (newVal) {
      selectedAvatarId.value = props.currentAvatarId || null;
    }
  }
);

function closeModal(): void {
  emit('update:show', false);
}

function selectAvatar(avatar: PredefinedAvatar): void {
  selectedAvatarId.value = avatar.slug;
}

function confirmSelection(): void {
  if (selectedAvatarId.value) {
    const avatar = avatars.value.find((a) => a.slug === selectedAvatarId.value);
    if (avatar) {
      emit('select', avatar);
    }
  }
  closeModal();
}

function handleBackdropClick(event: MouseEvent): void {
  if ((event.target as HTMLElement).classList.contains('modal-backdrop')) {
    closeModal();
  }
}
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="show" class="modal-backdrop" @click="handleBackdropClick">
        <div class="modal-content">
          <div class="modal-header">
            <h3 class="modal-title">{{ t('profile.selectAvatar') }}</h3>
            <button class="close-btn" @click="closeModal" :aria-label="t('common.close')">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="18" y1="6" x2="6" y2="18" />
                <line x1="6" y1="6" x2="18" y2="18" />
              </svg>
            </button>
          </div>

          <div class="modal-body">
            <p class="modal-description">{{ t('profile.selectAvatarDescription') }}</p>

            <div v-if="loading" class="avatar-loading">
              <div class="spinner" />
            </div>

            <div v-else class="avatar-grid">
              <button
                v-for="avatar in avatars"
                :key="avatar.id"
                class="avatar-option"
                :class="{ selected: selectedAvatarId === avatar.slug }"
                :style="{ backgroundColor: avatar.bg_color }"
                @click="selectAvatar(avatar)"
                :aria-label="avatar.display_name"
              >
                <img :src="avatar.path" :alt="avatar.display_name" class="avatar-image" />
                <div class="avatar-check" v-if="selectedAvatarId === avatar.slug">
                  <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
                    <polyline points="20 6 9 17 4 12" />
                  </svg>
                </div>
              </button>
            </div>
          </div>

          <div class="modal-footer">
            <button class="btn btn-secondary" @click="closeModal">
              {{ t('common.cancel') }}
            </button>
            <button
              class="btn btn-primary"
              @click="confirmSelection"
              :disabled="!selectedAvatarId || loading"
            >
              {{ t('common.save') }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.modal-backdrop {
  position: fixed;
  inset: 0;
  background-color: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: var(--spacing-lg);
}

.modal-content {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  width: 100%;
  max-width: 480px;
  max-height: 90vh;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  box-shadow: var(--shadow-lg);
}

.modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
}

.modal-title {
  font-size: var(--font-size-lg);
  font-weight: 600;
  margin: 0;
}

.close-btn {
  background: none;
  border: none;
  cursor: pointer;
  padding: var(--spacing-xs);
  color: var(--color-text-secondary);
  border-radius: var(--radius-sm);
  transition: color var(--transition-fast), background-color var(--transition-fast);
}

.close-btn:hover {
  color: var(--color-text-primary);
  background-color: var(--color-bg-secondary);
}

.modal-body {
  padding: var(--spacing-lg);
  overflow-y: auto;
}

.modal-description {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-lg);
  text-align: center;
}

.avatar-loading {
  display: flex;
  justify-content: center;
  padding: var(--spacing-xl);
}

.spinner {
  width: 32px;
  height: 32px;
  border: 3px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.avatar-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--spacing-md);
}

.avatar-option {
  position: relative;
  aspect-ratio: 1;
  border: 3px solid transparent;
  border-radius: var(--radius-lg);
  cursor: pointer;
  overflow: hidden;
  transition: transform var(--transition-fast), border-color var(--transition-fast), box-shadow var(--transition-fast);
  padding: 0;
}

.avatar-option:hover {
  transform: scale(1.05);
  box-shadow: var(--shadow-md);
}

.avatar-option.selected {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px var(--color-primary-light);
}

.avatar-image {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.avatar-check {
  position: absolute;
  bottom: var(--spacing-xs);
  right: var(--spacing-xs);
  width: 28px;
  height: 28px;
  background-color: var(--color-primary);
  border-radius: var(--radius-full);
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
}

[dir="rtl"] .avatar-check {
  right: auto;
  left: var(--spacing-xs);
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-md);
  padding: var(--spacing-lg);
  border-top: 1px solid var(--color-border);
}

/* Modal transitions */
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.2s ease;
}

.modal-enter-active .modal-content,
.modal-leave-active .modal-content {
  transition: transform 0.2s ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-from .modal-content,
.modal-leave-to .modal-content {
  transform: scale(0.95);
}

@media (max-width: 480px) {
  .avatar-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .modal-content {
    max-width: 100%;
  }
}
</style>
