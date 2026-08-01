<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { t } from '@/i18n';
import { useToast } from '@/composables/useToast';
import { useAuthStore } from '@/stores/auth';
import {
  listAdminAvatars,
  createAvatar,
  updateAvatar,
  replaceAvatarImage,
  deleteAvatar,
  reorderAvatars,
  type AdminAvatar,
} from '@/api/avatars';

const toast = useToast();
const authStore = useAuthStore();

const avatars = ref<AdminAvatar[]>([]);
const loading = ref(true);
const error = ref<string | null>(null);

// Modal state
const showCreateModal = ref(false);
const showEditModal = ref(false);
const showDeleteModal = ref(false);
const editingAvatar = ref<AdminAvatar | null>(null);
const deletingAvatar = ref<AdminAvatar | null>(null);
const submitting = ref(false);

// Create form
const createForm = ref({
  slug: '',
  display_name: '',
  category: 'animal',
  bg_color: '#2a2a3a',
  image: null as File | null,
});

// Edit form
const editForm = ref({
  display_name: '',
  category: '',
  bg_color: '',
});

// Replace image
const imagePreviewUrl = ref<string | null>(null);
const replacingImageId = ref<string | null>(null);

const canManage = computed(() => authStore.hasPermission('settings.manage'));

const categories = computed(() => [
  { value: 'animal', label: t('adminAvatars.categoryAnimal') },
  { value: 'character', label: t('adminAvatars.categoryCharacter') },
  { value: 'special', label: t('adminAvatars.categorySpecial') },
]);

// Drag & drop state
const dragIndex = ref<number | null>(null);
const dragOverIndex = ref<number | null>(null);

async function fetchAvatars(): Promise<void> {
  loading.value = true;
  error.value = null;
  try {
    const response = await listAdminAvatars();
    avatars.value = response.avatars || [];
  } catch (err: any) {
    error.value = err.response?.data?.error || t('common.error');
  } finally {
    loading.value = false;
  }
}

onMounted(fetchAvatars);

// Create
function openCreateModal(): void {
  // Cleanup previous preview
  if (imagePreviewUrl.value) {
    URL.revokeObjectURL(imagePreviewUrl.value);
    imagePreviewUrl.value = null;
  }
  createForm.value = { slug: '', display_name: '', category: 'animal', bg_color: '#2a2a3a', image: null };
  showCreateModal.value = true;
}

function handleImageSelect(event: Event): void {
  const input = event.target as HTMLInputElement;
  if (input.files && input.files[0]) {
    // Revoke previous blob URL if exists
    if (imagePreviewUrl.value) {
      URL.revokeObjectURL(imagePreviewUrl.value);
    }
    createForm.value.image = input.files[0];
    imagePreviewUrl.value = URL.createObjectURL(input.files[0]);
  }
}

async function handleCreate(): Promise<void> {
  if (!createForm.value.slug || !createForm.value.display_name || !createForm.value.image) {
    toast.error(t('avatars.fillRequired'));
    return;
  }
  submitting.value = true;
  try {
    const formData = new FormData();
    formData.append('slug', createForm.value.slug);
    formData.append('display_name', createForm.value.display_name);
    formData.append('category', createForm.value.category);
    formData.append('bg_color', createForm.value.bg_color);
    formData.append('image', createForm.value.image);
    await createAvatar(formData);
    toast.success(t('avatars.created'));
    showCreateModal.value = false;
    await fetchAvatars();
  } catch (err: any) {
    const message = err.response?.data?.error || t('common.error');
    toast.error(message);
  } finally {
    submitting.value = false;
  }
}

// Edit
function openEditModal(avatar: AdminAvatar): void {
  editingAvatar.value = avatar;
  editForm.value = {
    display_name: avatar.display_name,
    category: avatar.category,
    bg_color: avatar.bg_color,
  };
  showEditModal.value = true;
}

async function handleEdit(): Promise<void> {
  if (!editingAvatar.value) return;
  submitting.value = true;
  try {
    await updateAvatar(editingAvatar.value.id, editForm.value);
    toast.success(t('avatars.updated'));
    showEditModal.value = false;
    await fetchAvatars();
  } catch (err: any) {
    const message = err.response?.data?.error || t('common.error');
    toast.error(message);
  } finally {
    submitting.value = false;
  }
}

// Toggle active
async function handleToggleActive(avatar: AdminAvatar): Promise<void> {
  try {
    await updateAvatar(avatar.id, { is_active: !avatar.is_active });
    toast.success(avatar.is_active ? t('avatars.deactivated') : t('avatars.activated'));
    await fetchAvatars();
  } catch (err: any) {
    toast.error(err.response?.data?.error || t('common.error'));
  }
}

// Replace image
function triggerReplaceImage(avatarId: string): void {
  replacingImageId.value = avatarId;
  const input = document.createElement('input');
  input.type = 'file';
  input.accept = 'image/jpeg,image/png,image/webp';
  input.onchange = async (e: Event) => {
    const target = e.target as HTMLInputElement;
    if (!target.files || !target.files[0]) return;
    try {
      const formData = new FormData();
      formData.append('image', target.files[0]);
      await replaceAvatarImage(avatarId, formData);
      toast.success(t('avatars.imageReplaced'));
      await fetchAvatars();
    } catch (err: any) {
      toast.error(err.response?.data?.error || t('common.error'));
    } finally {
      replacingImageId.value = null;
    }
  };
  input.click();
}

// Delete
function openDeleteModal(avatar: AdminAvatar): void {
  deletingAvatar.value = avatar;
  showDeleteModal.value = true;
}

async function handleDelete(): Promise<void> {
  if (!deletingAvatar.value) return;
  submitting.value = true;
  try {
    await deleteAvatar(deletingAvatar.value.id);
    toast.success(t('avatars.deleted'));
    showDeleteModal.value = false;
    deletingAvatar.value = null;
    await fetchAvatars();
  } catch (err: any) {
    toast.error(err.response?.data?.error || t('common.error'));
  } finally {
    submitting.value = false;
  }
}

// Drag & Drop reorder
function handleDragStart(index: number): void {
  dragIndex.value = index;
}

function handleDragOver(event: DragEvent, index: number): void {
  event.preventDefault();
  dragOverIndex.value = index;
}

function handleDragLeave(): void {
  dragOverIndex.value = null;
}

async function handleDrop(targetIndex: number): Promise<void> {
  if (dragIndex.value === null || dragIndex.value === targetIndex) {
    dragIndex.value = null;
    dragOverIndex.value = null;
    return;
  }

  const items = [...avatars.value];
  const [moved] = items.splice(dragIndex.value, 1);
  items.splice(targetIndex, 0, moved);

  // Update local state immediately for responsiveness
  avatars.value = items;
  dragIndex.value = null;
  dragOverIndex.value = null;

  // Send reorder to backend
  try {
    const order = items.map((a, i) => ({ id: a.id, sort_order: i + 1 }));
    await reorderAvatars(order);
    toast.success(t('avatars.reordered'));
  } catch (err: any) {
    toast.error(err.response?.data?.error || t('common.error'));
    await fetchAvatars(); // Revert on error
  }
}

function handleDragEnd(): void {
  dragIndex.value = null;
  dragOverIndex.value = null;
}

function handleBackdropClick(event: MouseEvent): void {
  if ((event.target as HTMLElement).classList.contains('modal-backdrop')) {
    showCreateModal.value = false;
    showEditModal.value = false;
    showDeleteModal.value = false;
  }
}
</script>

<template>
  <div class="page">
    <div class="page-header">
      <div>
        <h1 class="page-title">{{ t('avatars.title') }}</h1>
        <p class="page-subtitle">{{ t('avatars.subtitle') }}</p>
      </div>
      <button v-if="canManage" class="btn btn-primary" @click="openCreateModal">
        + {{ t('avatars.addAvatar') }}
      </button>
    </div>

    <!-- Loading state -->
    <div v-if="loading" class="loading-container">
      <div class="spinner" />
    </div>

    <!-- Error state -->
    <div v-else-if="error" class="error-container">
      <p>{{ error }}</p>
      <button class="btn btn-secondary" @click="fetchAvatars">{{ t('common.retry') }}</button>
    </div>

    <!-- Empty state -->
    <div v-else-if="avatars.length === 0" class="empty-state">
      <p>{{ t('avatars.empty') }}</p>
    </div>

    <!-- Avatar grid -->
    <div v-else class="avatar-grid">
      <div
        v-for="(avatar, index) in avatars"
        :key="avatar.id"
        class="avatar-card"
        :class="{
          inactive: !avatar.is_active,
          dragging: dragIndex === index,
          'drag-over': dragOverIndex === index,
        }"
        :draggable="canManage"
        @dragstart="handleDragStart(index)"
        @dragover="handleDragOver($event, index)"
        @dragleave="handleDragLeave"
        @drop="handleDrop(index)"
        @dragend="handleDragEnd"
      >
        <div class="avatar-preview" :style="{ backgroundColor: avatar.bg_color }">
          <img :src="avatar.image_path" :alt="avatar.display_name" class="avatar-image" />
          <span v-if="!avatar.is_active" class="inactive-badge">{{ t('avatars.inactive') }}</span>
        </div>
        <div class="avatar-info">
          <div class="avatar-name">{{ avatar.display_name }}</div>
          <div class="avatar-meta">
            <span class="avatar-slug">{{ avatar.slug }}</span>
            <span class="avatar-category">{{ avatar.category }}</span>
          </div>
        </div>
        <div v-if="canManage" class="avatar-actions">
          <button class="action-btn" :title="t('common.edit')" @click="openEditModal(avatar)">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
              <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />
            </svg>
          </button>
          <button class="action-btn" :title="t('avatars.replaceImage')" @click="triggerReplaceImage(avatar.id)">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
              <circle cx="8.5" cy="8.5" r="1.5" />
              <polyline points="21 15 16 10 5 21" />
            </svg>
          </button>
          <button
            class="action-btn"
            :title="avatar.is_active ? t('avatars.deactivate') : t('avatars.activate')"
            @click="handleToggleActive(avatar)"
          >
            <svg v-if="avatar.is_active" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
              <circle cx="12" cy="12" r="3" />
            </svg>
            <svg v-else width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94" />
              <path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19" />
              <line x1="1" y1="1" x2="23" y2="23" />
            </svg>
          </button>
          <button class="action-btn action-btn-danger" :title="t('common.delete')" @click="openDeleteModal(avatar)">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="3 6 5 6 21 6" />
              <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
            </svg>
          </button>
        </div>
      </div>
    </div>

    <!-- Create Modal -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="showCreateModal" class="modal-backdrop" @click="handleBackdropClick">
          <div class="modal-content">
            <div class="modal-header">
              <h3>{{ t('avatars.addAvatar') }}</h3>
              <button class="close-btn" @click="showCreateModal = false">&times;</button>
            </div>
            <div class="modal-body">
              <div class="form-group">
                <label>{{ t('avatars.slug') }} *</label>
                <input v-model="createForm.slug" type="text" class="form-input" :placeholder="t('adminAvatars.slugPlaceholder')" pattern="[a-z0-9_-]+" />
                <span class="form-hint">{{ t('avatars.slugHint') }}</span>
              </div>
              <div class="form-group">
                <label>{{ t('avatars.displayName') }} *</label>
                <input v-model="createForm.display_name" type="text" class="form-input" :placeholder="t('adminAvatars.displayNamePlaceholder')" />
              </div>
              <div class="form-group">
                <label>{{ t('avatars.category') }}</label>
                <select v-model="createForm.category" class="form-input">
                  <option v-for="cat in categories" :key="cat.value" :value="cat.value">{{ cat.label }}</option>
                </select>
              </div>
              <div class="form-group">
                <label>{{ t('avatars.bgColor') }}</label>
                <div class="color-input-row">
                  <input v-model="createForm.bg_color" type="color" class="color-picker" />
                  <input v-model="createForm.bg_color" type="text" class="form-input" />
                </div>
              </div>
              <div class="form-group">
                <label>{{ t('avatars.image') }} *</label>
                <input type="file" accept="image/jpeg,image/png,image/webp" class="form-input" @change="handleImageSelect" />
                <span class="form-hint">{{ t('avatars.imageHint') }}</span>
              </div>
              <div v-if="imagePreviewUrl" class="image-preview" :style="{ backgroundColor: createForm.bg_color }">
                <img :src="imagePreviewUrl" alt="Preview" />
              </div>
            </div>
            <div class="modal-footer">
              <button class="btn btn-secondary" @click="showCreateModal = false">{{ t('common.cancel') }}</button>
              <button class="btn btn-primary" :disabled="submitting" @click="handleCreate">
                {{ submitting ? t('common.saving') : t('common.create') }}
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- Edit Modal -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="showEditModal && editingAvatar" class="modal-backdrop" @click="handleBackdropClick">
          <div class="modal-content">
            <div class="modal-header">
              <h3>{{ t('avatars.editAvatar') }}</h3>
              <button class="close-btn" @click="showEditModal = false">&times;</button>
            </div>
            <div class="modal-body">
              <div class="form-group">
                <label>{{ t('avatars.displayName') }}</label>
                <input v-model="editForm.display_name" type="text" class="form-input" />
              </div>
              <div class="form-group">
                <label>{{ t('avatars.category') }}</label>
                <select v-model="editForm.category" class="form-input">
                  <option v-for="cat in categories" :key="cat.value" :value="cat.value">{{ cat.label }}</option>
                </select>
              </div>
              <div class="form-group">
                <label>{{ t('avatars.bgColor') }}</label>
                <div class="color-input-row">
                  <input v-model="editForm.bg_color" type="color" class="color-picker" />
                  <input v-model="editForm.bg_color" type="text" class="form-input" />
                </div>
              </div>
            </div>
            <div class="modal-footer">
              <button class="btn btn-secondary" @click="showEditModal = false">{{ t('common.cancel') }}</button>
              <button class="btn btn-primary" :disabled="submitting" @click="handleEdit">
                {{ submitting ? t('common.saving') : t('common.save') }}
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- Delete Confirmation Modal -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="showDeleteModal && deletingAvatar" class="modal-backdrop" @click="handleBackdropClick">
          <div class="modal-content modal-sm">
            <div class="modal-header">
              <h3>{{ t('avatars.confirmDelete') }}</h3>
              <button class="close-btn" @click="showDeleteModal = false">&times;</button>
            </div>
            <div class="modal-body">
              <p>{{ t('avatars.confirmDeleteMsg', { name: deletingAvatar.display_name }) }}</p>
            </div>
            <div class="modal-footer">
              <button class="btn btn-secondary" @click="showDeleteModal = false">{{ t('common.cancel') }}</button>
              <button class="btn btn-danger" :disabled="submitting" @click="handleDelete">
                {{ submitting ? t('common.deleting') : t('common.delete') }}
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.page {
  padding: var(--spacing-lg);
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: var(--spacing-xl);
}

.page-title {
  font-size: var(--font-size-xl);
  font-weight: 700;
  margin: 0;
}

.page-subtitle {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin-top: var(--spacing-xs);
}

.loading-container {
  display: flex;
  justify-content: center;
  padding: var(--spacing-xxl);
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

.error-container {
  text-align: center;
  padding: var(--spacing-xxl);
  color: var(--color-error);
}

.empty-state {
  text-align: center;
  padding: var(--spacing-xxl);
  color: var(--color-text-secondary);
}

.avatar-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: var(--spacing-lg);
}

.avatar-card {
  background-color: var(--color-bg-secondary);
  border-radius: var(--radius-lg);
  overflow: hidden;
  border: 2px solid transparent;
  transition: border-color var(--transition-fast), opacity var(--transition-fast), transform var(--transition-fast);
  cursor: grab;
}

.avatar-card:hover {
  border-color: var(--color-border);
}

.avatar-card.inactive {
  opacity: 0.6;
}

.avatar-card.dragging {
  opacity: 0.4;
  transform: scale(0.95);
}

.avatar-card.drag-over {
  border-color: var(--color-primary);
}

.avatar-preview {
  position: relative;
  aspect-ratio: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}

.avatar-image {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.inactive-badge {
  position: absolute;
  top: var(--spacing-sm);
  right: var(--spacing-sm);
  background-color: var(--color-error);
  color: white;
  font-size: var(--font-size-xs);
  padding: 2px 6px;
  border-radius: var(--radius-sm);
}

[dir="rtl"] .inactive-badge {
  right: auto;
  left: var(--spacing-sm);
}

.avatar-info {
  padding: var(--spacing-sm) var(--spacing-md);
}

.avatar-name {
  font-weight: 600;
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
}

.avatar-meta {
  display: flex;
  gap: var(--spacing-sm);
  margin-top: var(--spacing-xs);
}

.avatar-slug {
  font-size: var(--font-size-xs);
  color: var(--color-text-tertiary);
  font-family: monospace;
}

.avatar-category {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  background: var(--color-bg-tertiary);
  padding: 1px 6px;
  border-radius: var(--radius-sm);
}

.avatar-actions {
  display: flex;
  gap: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-md) var(--spacing-sm);
}

.action-btn {
  background: none;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  padding: var(--spacing-xs);
  cursor: pointer;
  color: var(--color-text-secondary);
  transition: all var(--transition-fast);
  display: flex;
  align-items: center;
  justify-content: center;
}

.action-btn:hover {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-primary);
}

.action-btn-danger:hover {
  background-color: var(--color-error-light, #fee2e2);
  color: var(--color-error);
  border-color: var(--color-error);
}

/* Modals */
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

.modal-sm {
  max-width: 400px;
}

.modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
}

.modal-header h3 {
  margin: 0;
  font-size: var(--font-size-lg);
  font-weight: 600;
}

.close-btn {
  background: none;
  border: none;
  font-size: 24px;
  cursor: pointer;
  color: var(--color-text-secondary);
  padding: 0;
  line-height: 1;
}

.modal-body {
  padding: var(--spacing-lg);
  overflow-y: auto;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-md);
  padding: var(--spacing-lg);
  border-top: 1px solid var(--color-border);
}

.form-group {
  margin-bottom: var(--spacing-md);
}

.form-group label {
  display: block;
  font-size: var(--font-size-sm);
  font-weight: 500;
  margin-bottom: var(--spacing-xs);
  color: var(--color-text-primary);
}

.form-input {
  width: 100%;
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background-color: var(--color-bg-secondary);
  color: var(--color-text-primary);
  font-size: var(--font-size-sm);
}

.form-input:focus {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px var(--color-primary-light);
}

.form-hint {
  font-size: var(--font-size-xs);
  color: var(--color-text-tertiary);
  margin-top: var(--spacing-xs);
  display: block;
}

.color-input-row {
  display: flex;
  gap: var(--spacing-sm);
  align-items: center;
}

.color-picker {
  width: 40px;
  height: 40px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: 2px;
  cursor: pointer;
  flex-shrink: 0;
}

.image-preview {
  margin-top: var(--spacing-md);
  border-radius: var(--radius-lg);
  overflow: hidden;
  aspect-ratio: 1;
  max-width: 120px;
}

.image-preview img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

/* Button styles */
.btn {
  padding: var(--spacing-sm) var(--spacing-lg);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  border: none;
  transition: all var(--transition-fast);
}

.btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.btn-primary {
  background-color: var(--color-primary);
  color: white;
}

.btn-primary:hover:not(:disabled) {
  opacity: 0.9;
}

.btn-secondary {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-primary);
}

.btn-secondary:hover:not(:disabled) {
  background-color: var(--color-bg-secondary);
}

.btn-danger {
  background-color: var(--color-error);
  color: white;
}

.btn-danger:hover:not(:disabled) {
  opacity: 0.9;
}

/* Modal transitions */
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.2s ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
    gap: var(--spacing-md);
  }

  .avatar-grid {
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  }
}
</style>
