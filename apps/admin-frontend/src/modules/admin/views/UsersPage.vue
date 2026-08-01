<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useToast } from '@/composables/useToast';
import { useAuthStore } from '@/stores/auth';
import { SensitiveAdminAction, withPasswordReauthentication } from '@/api/reauthentication';
import { getUsers, getUser, updateUserRoles, updateUserStatus, createUser, type User, type UserDetailLegacy, type CreateUserRequest } from '@/api/users';

const router = useRouter();
const toast = useToast();
const auth = useAuthStore();

// Permission helpers
const canEditUsers = computed(() => auth.hasPermission('users.edit'));

// State
const users = ref<User[]>([]);
const selectedUser = ref<UserDetailLegacy | null>(null);
const loading = ref(true);
const detailLoading = ref(false);
const total = ref(0);
const page = ref(1);
const limit = 20;

// Filters
const searchQuery = ref('');
const roleFilter = ref('');
const statusFilter = ref('');

// Modals
const showRoleModal = ref(false);
const showStatusModal = ref(false);
const modalLoading = ref(false);
const selectedRoles = ref<string[]>([]);
const newStatus = ref<'active' | 'suspended'>('active');
const statusReason = ref('');

// Create user modal
const showCreateModal = ref(false);
const createForm = ref<CreateUserRequest>({
  email: '',
  password: '',
  display_name: '',
  roles: ['user'],
  email_verified: false,
});
const createdPassword = ref<string | null>(null);
const isCreating = ref(false);

const roles = ['user', 'support_admin', 'super_admin'];
const availableRoles = computed(() => {
  if (auth.isSuperAdmin) return roles;
  return ['user'];
});
const statuses = ['active', 'suspended'];

const offset = computed(() => (page.value - 1) * limit);
const totalPages = computed(() => Math.ceil(total.value / limit));

async function fetchUsers(): Promise<void> {
  loading.value = true;
  try {
    const response = await getUsers({
      limit,
      offset: offset.value,
      search: searchQuery.value || undefined,
      role: roleFilter.value || undefined,
      status: statusFilter.value || undefined,
    });
    users.value = response.users || [];
    total.value = response.total;
  } catch {
    toast.error(t('common.error'));
    users.value = [];
    total.value = 0;
  } finally {
    loading.value = false;
  }
}

async function selectUser(userId: string): Promise<void> {
  detailLoading.value = true;
  try {
    const detail = await getUser(userId);
    // Transform UserDetail to UserDetailLegacy format
    selectedUser.value = {
      id: detail.user.id,
      email: detail.user.email,
      roles: detail.roles,
      status: detail.user.status,
      kyc_status: detail.kyc.status,
      contest_count: detail.stats.total_contests,
      total_pnl: detail.stats.total_pnl,
      created_at: detail.user.created_at,
    };
  } catch {
    // Use data from list as fallback
    const user = users.value.find(u => u.id === userId);
    if (user) {
      selectedUser.value = {
        ...user,
        contest_count: 0,
        total_pnl: 0,
        created_at: user.created_at,
      };
    }
  } finally {
    detailLoading.value = false;
  }
}

function openRoleModal(): void {
  if (!selectedUser.value) return;
  selectedRoles.value = [...selectedUser.value.roles];
  showRoleModal.value = true;
}

function openStatusModal(status: 'active' | 'suspended'): void {
  if (!selectedUser.value) return;
  newStatus.value = status;
  statusReason.value = '';
  showStatusModal.value = true;
}

async function saveRoles(): Promise<void> {
  if (!selectedUser.value) return;
  modalLoading.value = true;
  try {
    const reason = window.prompt('Reason for this privileged role change:')?.trim();
    const password = window.prompt('Confirm your current Admin password:') || '';
    if (!reason || !password) return;
    await withPasswordReauthentication({
      password, action: SensitiveAdminAction.UserRolesUpdate, resourceId: selectedUser.value.id,
    }, grant => updateUserRoles(selectedUser.value!.id, { roles: selectedRoles.value, reason }, grant));
    selectedUser.value.roles = [...selectedRoles.value];
    // Update in list too
    const userInList = users.value.find(u => u.id === selectedUser.value?.id);
    if (userInList) {
      userInList.roles = [...selectedRoles.value];
    }
    toast.success(t('users.rolesUpdated'));
    showRoleModal.value = false;
  } catch {
    toast.error(t('users.rolesUpdateError'));
  } finally {
    modalLoading.value = false;
  }
}

async function saveStatus(): Promise<void> {
  if (!selectedUser.value) return;
  modalLoading.value = true;
  try {
    await updateUserStatus(selectedUser.value.id, { status: newStatus.value, reason: statusReason.value });
    selectedUser.value.status = newStatus.value;
    // Update in list too
    const userInList = users.value.find(u => u.id === selectedUser.value?.id);
    if (userInList) {
      userInList.status = newStatus.value;
    }
    toast.success(t('users.statusUpdated'));
    showStatusModal.value = false;
  } catch {
    toast.error(t('users.statusUpdateError'));
  } finally {
    modalLoading.value = false;
  }
}

function toggleRole(role: string): void {
  const idx = selectedRoles.value.indexOf(role);
  if (idx === -1) {
    selectedRoles.value.push(role);
  } else {
    selectedRoles.value.splice(idx, 1);
  }
}

function openCreateModal(): void {
  createForm.value = {
    email: '',
    password: '',
    display_name: '',
    roles: ['user'],
    email_verified: false,
  };
  createdPassword.value = null;
  showCreateModal.value = true;
}

function toggleCreateRole(role: string): void {
  const idx = createForm.value.roles.indexOf(role);
  if (idx >= 0) {
    if (createForm.value.roles.length > 1) {
      createForm.value.roles.splice(idx, 1);
    }
  } else {
    createForm.value.roles.push(role);
  }
}

async function submitCreateUser(): Promise<void> {
  if (!createForm.value.email) {
    toast.error(t('users.emailRequired'));
    return;
  }
  isCreating.value = true;
  try {
    const elevated = createForm.value.roles.some(role => role === 'support_admin' || role === 'super_admin');
    let result;
    if (elevated) {
      const reason = window.prompt('Reason for creating this privileged Admin account:')?.trim();
      const password = window.prompt('Confirm your current Admin password:') || '';
      if (!reason || !password) return;
      const resourceId = createForm.value.email.trim().toLowerCase();
      result = await withPasswordReauthentication({
        password, action: SensitiveAdminAction.ElevatedUserCreate, resourceId,
      }, grant => createUser({ ...createForm.value, reason }, grant));
    } else {
      result = await createUser(createForm.value);
    }
    if (result.temporary_password) {
      createdPassword.value = result.temporary_password;
    } else {
      showCreateModal.value = false;
      toast.success(t('users.createSuccess'));
    }
    await fetchUsers();
  } catch (err: unknown) {
    const axiosErr = err as { response?: { data?: { error?: string } } };
    const msg = axiosErr?.response?.data?.error || t('users.createError');
    toast.error(msg);
  } finally {
    isCreating.value = false;
  }
}

function copyPassword(): void {
  if (createdPassword.value) {
    navigator.clipboard.writeText(createdPassword.value);
    toast.success(t('users.passwordCopied'));
  }
}

function closeCreateModal(): void {
  showCreateModal.value = false;
  createdPassword.value = null;
}

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString();
}

function getStatusClass(status: string): string {
  const classes: Record<string, string> = {
    active: 'status-active',
    suspended: 'status-suspended',
    pending: 'status-pending',
  };
  return classes[status] || 'status-default';
}

function getKycStatusClass(status: string | undefined): string {
  if (!status) return 'kyc-none';
  const classes: Record<string, string> = {
    verified: 'kyc-verified',
    pending: 'kyc-pending',
    rejected: 'kyc-rejected',
    none: 'kyc-none',
  };
  return classes[status] || 'kyc-none';
}

function getRoleBadgeClass(role: string): string {
  const classes: Record<string, string> = {
    admin: 'role-admin',
    moderator: 'role-moderator',
    user: 'role-user',
    viewer: 'role-viewer',
    super_admin: 'role-super-admin',
  };
  return classes[role] || 'role-user';
}

function viewUserDetail(userId: string): void {
  router.push(`/admin/users/${userId}`);
}

// Debounced search
let searchTimeout: ReturnType<typeof setTimeout>;
watch(searchQuery, () => {
  clearTimeout(searchTimeout);
  searchTimeout = setTimeout(() => {
    page.value = 1;
    fetchUsers();
  }, 300);
});

watch([roleFilter, statusFilter], () => {
  page.value = 1;
  fetchUsers();
});

watch(page, fetchUsers);

onMounted(fetchUsers);
</script>

<template>
  <div class="users-page">
    <div class="page-header">
      <h1 class="page-title">{{ t('users.title') }}</h1>
      <button v-if="canEditUsers" class="btn btn-primary" @click="openCreateModal">
        + {{ t('users.createUser') }}
      </button>
    </div>

    <div class="filters">
      <input
        v-model="searchQuery"
        type="text"
        class="input search-input"
        :placeholder="t('users.search')"
      />
      <select v-model="roleFilter" class="input filter-select">
        <option value="">{{ t('users.allRoles') }}</option>
        <option v-for="role in roles" :key="role" :value="role">
          {{ t(`users.role.${role}`) }}
        </option>
      </select>
      <select v-model="statusFilter" class="input filter-select">
        <option value="">{{ t('users.allStatuses') }}</option>
        <option v-for="status in statuses" :key="status" :value="status">
          {{ t(`users.status.${status}`) }}
        </option>
      </select>
    </div>

    <div class="content-layout">
      <!-- Users List -->
      <div class="users-list-section">
        <div v-if="loading" class="loading">
          {{ t('common.loading') }}
        </div>

        <div v-else-if="users.length === 0" class="no-results">
          {{ t('users.noResults') }}
        </div>

        <div v-else class="users-list">
          <div
            v-for="user in users"
            :key="user.id"
            :class="['user-card', { selected: selectedUser?.id === user.id }]"
            @click="selectUser(user.id)"
          >
            <div class="user-card-header">
              <span class="user-email">{{ user.email }}</span>
              <span :class="['status-badge', getStatusClass(user.status)]">
                {{ t(`users.status.${user.status}`) }}
              </span>
            </div>
            <div class="user-card-meta">
              <div class="user-roles">
                <span
                  v-for="role in user.roles"
                  :key="role"
                  :class="['role-badge', getRoleBadgeClass(role)]"
                >
                  {{ t(`users.role.${role}`) }}
                </span>
              </div>
              <span class="user-date">{{ formatDate(user.created_at) }}</span>
            </div>
          </div>

          <!-- Pagination -->
          <div v-if="totalPages > 1" class="pagination">
            <button
              class="btn btn-ghost btn-sm"
              :disabled="page <= 1"
              @click="page--"
            >
              {{ t('common.previous') }}
            </button>
            <span class="page-info">{{ page }} / {{ totalPages }}</span>
            <button
              class="btn btn-ghost btn-sm"
              :disabled="page >= totalPages"
              @click="page++"
            >
              {{ t('common.next') }}
            </button>
          </div>
        </div>
      </div>

      <!-- User Detail Panel -->
      <div class="user-detail-section">
        <div v-if="!selectedUser" class="select-prompt">
          {{ t('users.selectUser') }}
        </div>

        <div v-else-if="detailLoading" class="loading">
          {{ t('common.loading') }}
        </div>

        <div v-else class="user-detail">
          <div class="detail-header">
            <h2 class="detail-title">{{ selectedUser.email }}</h2>
            <span :class="['status-badge', getStatusClass(selectedUser.status)]">
              {{ t(`users.status.${selectedUser.status}`) }}
            </span>
          </div>

          <div class="detail-section">
            <h3 class="section-title">{{ t('users.info') }}</h3>
            <div class="info-grid">
              <div class="info-item">
                <span class="info-label">{{ t('users.id') }}</span>
                <span class="info-value id-value">{{ selectedUser.id }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">{{ t('users.email') }}</span>
                <span class="info-value">{{ selectedUser.email }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">{{ t('users.createdAt') }}</span>
                <span class="info-value">{{ formatDate(selectedUser.created_at) }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">{{ t('users.kycStatus') }}</span>
                <span :class="['info-value', 'kyc-badge', getKycStatusClass(selectedUser.kyc_status)]">
                  {{ t(`users.kyc.${selectedUser.kyc_status || 'none'}`) }}
                </span>
              </div>
              <div class="info-item">
                <span class="info-label">{{ t('users.contestCount') }}</span>
                <span class="info-value">{{ selectedUser.contest_count }}</span>
              </div>
              <div class="info-item">
                <span class="info-label">{{ t('users.totalPnl') }}</span>
                <span :class="['info-value', selectedUser.total_pnl >= 0 ? 'pnl-positive' : 'pnl-negative']">
                  {{ selectedUser.total_pnl >= 0 ? '+' : '' }}{{ selectedUser.total_pnl.toFixed(2) }}
                </span>
              </div>
            </div>
          </div>

          <div class="detail-section">
            <div class="section-header">
              <h3 class="section-title">{{ t('users.roles') }}</h3>
              <button
                v-if="canEditUsers"
                class="btn btn-ghost btn-sm"
                @click="openRoleModal"
              >
                {{ t('users.editRoles') }}
              </button>
            </div>
            <div class="roles-list">
              <span
                v-for="role in selectedUser.roles"
                :key="role"
                :class="['role-badge', 'role-large', getRoleBadgeClass(role)]"
              >
                {{ t(`users.role.${role}`) }}
              </span>
            </div>
          </div>

          <div class="detail-section">
            <h3 class="section-title">{{ t('users.actions') }}</h3>
            <div class="action-buttons">
              <button
                class="btn btn-primary"
                @click="viewUserDetail(selectedUser.id)"
              >
                {{ t('users.viewDetails') }}
              </button>
              <button
                v-if="canEditUsers && selectedUser.status === 'active'"
                class="btn btn-danger"
                @click="openStatusModal('suspended')"
              >
                {{ t('users.suspend') }}
              </button>
              <button
                v-if="canEditUsers && selectedUser.status !== 'active'"
                class="btn btn-success"
                @click="openStatusModal('active')"
              >
                {{ t('users.activate') }}
              </button>
            </div>
          </div>

          <!-- Viewer mode notice -->
          <div v-if="!canEditUsers" class="viewer-notice">
            <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
              <path d="M8 1a7 7 0 100 14A7 7 0 008 1zm0 12.5a5.5 5.5 0 110-11 5.5 5.5 0 010 11z"/>
              <path d="M8 4.5a.75.75 0 01.75.75v3a.75.75 0 01-1.5 0v-3A.75.75 0 018 4.5zm0 6a.75.75 0 110 1.5.75.75 0 010-1.5z"/>
            </svg>
            <span>{{ t('rbac.viewOnlyMode') }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Role Modal -->
    <Teleport to="body">
      <div v-if="showRoleModal" class="modal-overlay" @click.self="showRoleModal = false">
        <div class="modal">
          <div class="modal-header">
            <h3 class="modal-title">{{ t('users.editRolesTitle') }}</h3>
            <button class="modal-close" @click="showRoleModal = false">&times;</button>
          </div>
          <div class="modal-body">
            <p class="modal-description">{{ t('users.editRolesDescription') }}</p>
            <div class="role-checkboxes">
              <label v-for="role in availableRoles" :key="role" class="role-checkbox">
                <input
                  type="checkbox"
                  :checked="selectedRoles.includes(role)"
                  @change="toggleRole(role)"
                />
                <span :class="['role-badge', 'role-large', getRoleBadgeClass(role)]">
                  {{ t(`users.role.${role}`) }}
                </span>
              </label>
            </div>
          </div>
          <div class="modal-footer">
            <button class="btn btn-ghost" @click="showRoleModal = false">
              {{ t('common.cancel') }}
            </button>
            <button class="btn btn-primary" :disabled="modalLoading || selectedRoles.length === 0" @click="saveRoles">
              {{ modalLoading ? t('common.loading') : t('common.save') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Status Modal -->
    <Teleport to="body">
      <div v-if="showStatusModal" class="modal-overlay" @click.self="showStatusModal = false">
        <div class="modal">
          <div class="modal-header">
            <h3 class="modal-title">
              {{ newStatus === 'suspended' ? t('users.suspendTitle') : t('users.activateTitle') }}
            </h3>
            <button class="modal-close" @click="showStatusModal = false">&times;</button>
          </div>
          <div class="modal-body">
            <p class="modal-description">
              {{ newStatus === 'suspended'
                ? t('users.suspendConfirmation', { email: selectedUser?.email ?? '' })
                : t('users.activateConfirmation', { email: selectedUser?.email ?? '' })
              }}
            </p>
            <div v-if="newStatus === 'suspended'" class="form-group">
              <label class="form-label">{{ t('users.suspendReason') }}</label>
              <textarea
                v-model="statusReason"
                class="input textarea"
                :placeholder="t('users.suspendReasonPlaceholder')"
                rows="3"
              />
            </div>
          </div>
          <div class="modal-footer">
            <button class="btn btn-ghost" @click="showStatusModal = false">
              {{ t('common.cancel') }}
            </button>
            <button
              :class="['btn', newStatus === 'suspended' ? 'btn-danger' : 'btn-success']"
              :disabled="modalLoading"
              @click="saveStatus"
            >
              {{ modalLoading ? t('common.loading') : t('common.confirm') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Create User Modal -->
    <Teleport to="body">
      <div v-if="showCreateModal" class="modal-overlay" @click.self="closeCreateModal">
        <div class="modal create-user-modal">
          <!-- Password Result View -->
          <template v-if="createdPassword">
            <div class="modal-header">
              <h3 class="modal-title">{{ t('users.userCreated') }}</h3>
              <button class="modal-close" @click="closeCreateModal">&times;</button>
            </div>
            <div class="modal-body">
              <p class="password-notice">{{ t('users.passwordNotice') }}</p>
              <div class="password-display">
                <code>{{ createdPassword }}</code>
                <button class="btn btn-ghost btn-sm" @click="copyPassword">
                  {{ t('users.copyPassword') }}
                </button>
              </div>
            </div>
            <div class="modal-footer">
              <button class="btn btn-primary" @click="closeCreateModal">
                {{ t('users.done') }}
              </button>
            </div>
          </template>

          <!-- Create Form View -->
          <template v-else>
            <div class="modal-header">
              <h3 class="modal-title">{{ t('users.createUser') }}</h3>
              <button class="modal-close" @click="closeCreateModal">&times;</button>
            </div>
            <div class="modal-body">
              <div class="form-group">
                <label class="form-label">{{ t('users.email') }} *</label>
                <input
                  v-model="createForm.email"
                  type="email"
                  class="input"
                  dir="ltr"
                  :placeholder="t('users.emailPlaceholder')"
                />
              </div>

              <div class="form-group">
                <label class="form-label">{{ t('users.displayName') }}</label>
                <input
                  v-model="createForm.display_name"
                  type="text"
                  class="input"
                  :placeholder="t('users.displayNamePlaceholder')"
                />
              </div>

              <div class="form-group">
                <label class="form-label">{{ t('users.password') }}</label>
                <input
                  v-model="createForm.password"
                  type="text"
                  class="input"
                  dir="ltr"
                  :placeholder="t('users.passwordAutoGenerate')"
                />
                <small class="form-hint">{{ t('users.passwordHint') }}</small>
              </div>

              <div class="form-group">
                <label class="form-label">{{ t('users.assignRoles') }}</label>
                <div class="role-chips">
                  <button
                    v-for="role in availableRoles"
                    :key="role"
                    :class="['role-chip', getRoleBadgeClass(role), { active: createForm.roles.includes(role) }]"
                    @click="toggleCreateRole(role)"
                  >
                    {{ t(`users.role.${role}`) }}
                  </button>
                </div>
              </div>

              <div class="form-group">
                <label class="checkbox-label">
                  <input v-model="createForm.email_verified" type="checkbox" />
                  {{ t('users.markEmailVerified') }}
                </label>
              </div>
            </div>
            <div class="modal-footer">
              <button class="btn btn-ghost" @click="closeCreateModal" :disabled="isCreating">
                {{ t('common.cancel') }}
              </button>
              <button class="btn btn-primary" @click="submitCreateUser" :disabled="isCreating">
                {{ isCreating ? t('common.loading') : t('users.createUser') }}
              </button>
            </div>
          </template>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.users-page {
  padding: var(--spacing-lg) 0;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-xl);
}

.page-title {
  font-size: var(--font-size-2xl);
  font-weight: 600;
  color: var(--color-text-primary);
}

.filters {
  display: flex;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
}

.search-input {
  flex: 1;
  max-width: 300px;
}

.filter-select {
  width: 160px;
}

.content-layout {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--spacing-xl);
  min-height: 500px;
}

.users-list-section,
.user-detail-section {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
  overflow: hidden;
}

.users-list {
  display: flex;
  flex-direction: column;
}

.user-card {
  padding: var(--spacing-md);
  border-bottom: 1px solid var(--color-border);
  cursor: pointer;
  transition: background-color var(--transition-fast);
}

.user-card:hover {
  background-color: var(--color-bg-secondary);
}

.user-card.selected {
  background-color: var(--color-primary-light);
  border-right: 3px solid var(--color-primary);
}

[dir="rtl"] .user-card.selected {
  border-right: none;
  border-left: 3px solid var(--color-primary);
}

.user-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-xs);
}

.user-email {
  font-weight: 500;
  color: var(--color-text-primary);
}

.user-card-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.user-roles {
  display: flex;
  gap: var(--spacing-xs);
}

.user-date {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

.status-badge {
  display: inline-block;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-full);
  font-size: var(--font-size-xs);
  font-weight: 600;
  text-transform: uppercase;
}

.status-active {
  background-color: #DCFCE7;
  color: #16A34A;
}

.status-suspended {
  background-color: #FEE2E2;
  color: #DC2626;
}

.status-pending {
  background-color: #FEF3C7;
  color: #D97706;
}

.role-badge {
  display: inline-block;
  padding: 2px 6px;
  border-radius: var(--radius-sm);
  font-size: var(--font-size-xs);
  font-weight: 500;
}

.role-large {
  padding: var(--spacing-xs) var(--spacing-sm);
}

.role-admin {
  background-color: #DBEAFE;
  color: #2563EB;
}

.role-moderator {
  background-color: #F3E8FF;
  color: #9333EA;
}

.role-user {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-secondary);
}

.role-viewer {
  background-color: #FEF3C7;
  color: #D97706;
}

.role-super-admin {
  background-color: #FEE2E2;
  color: #DC2626;
}

.viewer-notice {
  padding: var(--spacing-md);
  background-color: var(--color-warning-light, #fef3c7);
  color: var(--color-warning, #d97706);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.kyc-badge {
  padding: 2px 8px;
  border-radius: var(--radius-sm);
  font-size: var(--font-size-sm);
}

.kyc-verified {
  background-color: #DCFCE7;
  color: #16A34A;
}

.kyc-pending {
  background-color: #FEF3C7;
  color: #D97706;
}

.kyc-rejected {
  background-color: #FEE2E2;
  color: #DC2626;
}

.kyc-none {
  background-color: var(--color-bg-tertiary);
  color: var(--color-text-muted);
}

.pagination {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-md);
  border-top: 1px solid var(--color-border);
}

.page-info {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.select-prompt,
.loading,
.no-results {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  min-height: 200px;
  color: var(--color-text-secondary);
  padding: var(--spacing-xl);
  text-align: center;
}

.user-detail {
  padding: var(--spacing-lg);
}

.detail-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-xl);
  padding-bottom: var(--spacing-md);
  border-bottom: 1px solid var(--color-border);
}

.detail-title {
  font-size: var(--font-size-xl);
  font-weight: 600;
  color: var(--color-text-primary);
}

.detail-section {
  margin-bottom: var(--spacing-xl);
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--spacing-md);
}

.section-title {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: var(--spacing-md);
}

.section-header .section-title {
  margin-bottom: 0;
}

.info-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--spacing-md);
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.info-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

.info-value {
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
}

.id-value {
  font-family: var(--font-family-mono);
  font-size: var(--font-size-xs);
  word-break: break-all;
}

.pnl-positive {
  color: #16A34A;
}

.pnl-negative {
  color: #DC2626;
}

.roles-list {
  display: flex;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}

.action-buttons {
  display: flex;
  gap: var(--spacing-md);
}

.btn-success {
  background-color: #16A34A;
  color: white;
}

.btn-success:hover {
  background-color: #15803D;
}

.btn-danger {
  background-color: #DC2626;
  color: white;
}

.btn-danger:hover {
  background-color: #B91C1C;
}

/* Modal Styles */
.modal-overlay {
  position: fixed;
  inset: 0;
  background-color: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal {
  background-color: var(--color-bg-primary);
  border-radius: var(--radius-lg);
  width: 100%;
  max-width: 400px;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: var(--shadow-lg);
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
}

.modal-close {
  background: none;
  border: none;
  font-size: 24px;
  color: var(--color-text-muted);
  cursor: pointer;
  padding: 0;
  line-height: 1;
}

.modal-close:hover {
  color: var(--color-text-primary);
}

.modal-body {
  padding: var(--spacing-lg);
}

.modal-description {
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-lg);
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-md);
  padding: var(--spacing-lg);
  border-top: 1px solid var(--color-border);
}

.role-checkboxes {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.role-checkbox {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  cursor: pointer;
}

.role-checkbox input {
  width: 18px;
  height: 18px;
  cursor: pointer;
}

.form-group {
  margin-bottom: var(--spacing-md);
}

.form-label {
  display: block;
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
  margin-bottom: var(--spacing-xs);
}

.textarea {
  resize: vertical;
  min-height: 80px;
}

/* Create User Modal */
.create-user-modal {
  max-width: 480px;
}

.role-chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-xs);
}

.role-chip {
  padding: 4px 12px;
  border-radius: var(--radius-full);
  border: 2px solid transparent;
  cursor: pointer;
  font-size: var(--font-size-sm);
  font-weight: 500;
  opacity: 0.5;
  transition: opacity var(--transition-fast);
}

.role-chip.active {
  opacity: 1;
  border-color: currentColor;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  cursor: pointer;
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
}

.checkbox-label input {
  width: 18px;
  height: 18px;
  cursor: pointer;
}

.password-display {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  background: var(--color-bg-secondary);
  border-radius: var(--radius-md);
  margin: var(--spacing-md) 0;
}

.password-display code {
  flex: 1;
  font-family: var(--font-family-mono);
  font-size: var(--font-size-lg);
  word-break: break-all;
  direction: ltr;
}

.password-notice {
  color: #D97706;
  font-weight: 500;
  font-size: var(--font-size-sm);
}

.form-hint {
  display: block;
  color: var(--color-text-muted);
  font-size: var(--font-size-xs);
  margin-top: var(--spacing-xs);
}

@media (max-width: 767px) {
  .filters {
    flex-direction: column;
  }

  .search-input,
  .filter-select {
    max-width: none;
    width: 100%;
  }

  .content-layout {
    grid-template-columns: 1fr;
  }

  .info-grid {
    grid-template-columns: 1fr;
  }
}
</style>
