<script setup lang="ts">
import { ref, computed, reactive, onMounted, watch } from 'vue';
import { t } from '@/i18n';
import { useAuthStore } from '@/stores/auth';
import { useToast } from '@/composables/useToast';
import {
  listEmailTemplates,
  listTemplateVersions,
  getTemplateVersion,
  createTemplateVersion,
  updateTemplateVersion,
  deleteTemplateVersion,
  activateTemplateVersion,
  type EmailTemplate,
  type TemplateVersionListItem,
  type TemplateVersionDetail,
  type FontConfig,
  type TemplateVersionsResponse,
} from '@/api/email-templates';

const auth = useAuthStore();
const toast = useToast();

// ─── Category definitions ───
const categories = computed(() => ({
  authentication: {
    label: t('emailTemplates.cat.authentication'),
    icon: '\uD83D\uDD10',
    slugs: ['welcome', 'email_verification', 'password_reset'],
  },
  kyc: {
    label: t('emailTemplates.cat.kyc'),
    icon: '\uD83D\uDCCB',
    slugs: ['kyc_approved', 'kyc_rejected', 'kyc_info_request'],
  },
  financial: {
    label: t('emailTemplates.cat.financial'),
    icon: '\uD83D\uDCB0',
    slugs: ['deposit_confirmed', 'withdrawal_approved', 'withdrawal_rejected', 'withdrawal_processing', 'withdrawal_completed'],
  },
  contests: {
    label: t('emailTemplates.cat.contests'),
    icon: '\uD83C\uDFC6',
    slugs: ['contest_starting', 'contest_cancelled', 'contest_summary', 'prize_won'],
  },
  system: {
    label: t('emailTemplates.cat.system'),
    icon: '\u2699\uFE0F',
    slugs: ['daily_digest', 'bug_report'],
  },
}));

// ─── State ───
const templates = ref<EmailTemplate[]>([]);
const loading = ref(true);

// Left panel
const expandedCategories = ref<Set<string>>(new Set(['authentication']));
const selectedSlug = ref<string | null>(null);

// Middle panel
const versions = ref<TemplateVersionListItem[]>([]);
const maxVersions = ref(5);
const loadingVersions = ref(false);
const selectedVersionId = ref<string | null>(null);

// Right panel
const selectedVersion = ref<TemplateVersionDetail | null>(null);
const loadingVersion = ref(false);
const isCreating = ref(false);
const editorTab = ref<'html' | 'css' | 'fonts'>('html');

const editForm = reactive({
  version_name: '',
  html_body: '',
  css_content: '',
  font_config: {
    en: { family: 'Inter', weight: '400', url: 'https://fonts.googleapis.com/css2?family=Inter:wght@400;600;700' },
    fa: { family: 'Vazirmatn', weight: '400', url: 'https://fonts.googleapis.com/css2?family=Vazirmatn:wght@400;600;700' },
  } as FontConfig,
});
const setActiveOnCreate = ref(false);
const isDirty = ref(false);
const saving = ref(false);

// Modals
const showDeleteModal = ref(false);
const showActivateModal = ref(false);
const showPreviewModal = ref(false);
const pendingActionVersionId = ref<string | null>(null);
const previewWidth = ref<'desktop' | 'mobile'>('desktop');
const previewLang = ref<'en' | 'fa'>('en');

// Mobile responsive
const mobilePanel = ref<'left' | 'middle' | 'right'>('left');

// ─── Permissions ───
const canEdit = computed(() => auth.hasPermission('settings.manage'));

// ─── Template helpers ───
function formatSlug(slug: string): string {
  return slug.replace(/_/g, ' ').replace(/\b\w/g, (l) => l.toUpperCase());
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
}

function formatVariable(variable: string): string {
  return `{{.${variable}}}`;
}

function getTemplateBySlug(slug: string): EmailTemplate | undefined {
  return templates.value.find((t) => t.slug === slug);
}

function fontSummary(fc: FontConfig): string {
  const parts: string[] = [];
  if (fc.en) parts.push(`EN: ${fc.en.family}`);
  if (fc.fa) parts.push(`FA: ${fc.fa.family}`);
  return parts.join(' | ');
}

// ─── Data Loading ───
async function loadTemplates() {
  loading.value = true;
  try {
    templates.value = await listEmailTemplates();
  } catch {
    toast.error(t('emailTemplates.loadError'));
  } finally {
    loading.value = false;
  }
}

async function loadVersions(slug: string) {
  loadingVersions.value = true;
  try {
    const resp: TemplateVersionsResponse = await listTemplateVersions(slug);
    versions.value = resp.versions || [];
    maxVersions.value = resp.max_versions || 5;
  } catch {
    toast.error(t('emailTemplates.loadError'));
    versions.value = [];
  } finally {
    loadingVersions.value = false;
  }
}

async function loadVersion(slug: string, versionId: string) {
  loadingVersion.value = true;
  try {
    selectedVersion.value = await getTemplateVersion(slug, versionId);
    populateEditForm(selectedVersion.value);
    isDirty.value = false;
    isCreating.value = false;
  } catch {
    toast.error(t('emailTemplates.loadError'));
    selectedVersion.value = null;
  } finally {
    loadingVersion.value = false;
  }
}

function populateEditForm(v: TemplateVersionDetail) {
  editForm.version_name = v.version_name;
  editForm.html_body = v.html_body;
  editForm.css_content = v.css_content;
  editForm.font_config = JSON.parse(JSON.stringify(v.font_config || {
    en: { family: 'Inter', weight: '400', url: 'https://fonts.googleapis.com/css2?family=Inter:wght@400;600;700' },
    fa: { family: 'Vazirmatn', weight: '400', url: 'https://fonts.googleapis.com/css2?family=Vazirmatn:wght@400;600;700' },
  }));
}

function resetEditForm() {
  editForm.version_name = '';
  editForm.html_body = '';
  editForm.css_content = '';
  editForm.font_config = {
    en: { family: 'Inter', weight: '400', url: 'https://fonts.googleapis.com/css2?family=Inter:wght@400;600;700' },
    fa: { family: 'Vazirmatn', weight: '400', url: 'https://fonts.googleapis.com/css2?family=Vazirmatn:wght@400;600;700' },
  };
  setActiveOnCreate.value = false;
  isDirty.value = false;
}

// ─── Actions ───
function toggleCategory(key: string) {
  if (expandedCategories.value.has(key)) {
    expandedCategories.value.delete(key);
  } else {
    expandedCategories.value.add(key);
  }
}

function selectSlug(slug: string) {
  if (isDirty.value) {
    if (!confirm(t('emailTemplates.unsavedChanges'))) return;
  }
  selectedSlug.value = slug;
  selectedVersionId.value = null;
  selectedVersion.value = null;
  isCreating.value = false;
  isDirty.value = false;
  editorTab.value = 'html';
  loadVersions(slug);
  mobilePanel.value = 'middle';
}

function selectVersion(versionId: string) {
  if (isDirty.value) {
    if (!confirm(t('emailTemplates.unsavedChanges'))) return;
  }
  if (!selectedSlug.value) return;
  selectedVersionId.value = versionId;
  isCreating.value = false;
  editorTab.value = 'html';
  loadVersion(selectedSlug.value, versionId);
  mobilePanel.value = 'right';
}

function startCreate() {
  if (isDirty.value) {
    if (!confirm(t('emailTemplates.unsavedChanges'))) return;
  }
  isCreating.value = true;
  selectedVersionId.value = null;
  selectedVersion.value = null;
  resetEditForm();
  editorTab.value = 'html';
  mobilePanel.value = 'right';
}

async function saveVersion() {
  if (!selectedSlug.value || !canEdit.value) return;
  saving.value = true;
  try {
    if (isCreating.value) {
      await createTemplateVersion(selectedSlug.value, {
        version_name: editForm.version_name,
        html_body: editForm.html_body,
        css_content: editForm.css_content,
        font_config: editForm.font_config,
        is_active: setActiveOnCreate.value,
      });
      toast.success(t('emailTemplates.saveSuccess'));
      isCreating.value = false;
    } else if (selectedVersionId.value) {
      await updateTemplateVersion(selectedSlug.value, selectedVersionId.value, {
        version_name: editForm.version_name,
        html_body: editForm.html_body,
        css_content: editForm.css_content,
        font_config: editForm.font_config,
      });
      toast.success(t('emailTemplates.saveSuccess'));
    }
    isDirty.value = false;
    await loadVersions(selectedSlug.value);
    // Select the newly created or updated version
    if (versions.value.length > 0 && isCreating.value === false && !selectedVersionId.value) {
      const latest = versions.value[versions.value.length - 1];
      selectVersion(latest.id);
    } else if (selectedVersionId.value) {
      await loadVersion(selectedSlug.value!, selectedVersionId.value);
    }
  } catch {
    toast.error(t('emailTemplates.saveError'));
  } finally {
    saving.value = false;
  }
}

function cancelEdit() {
  if (isDirty.value) {
    if (!confirm(t('emailTemplates.unsavedChanges'))) return;
  }
  if (isCreating.value) {
    isCreating.value = false;
    selectedVersion.value = null;
    resetEditForm();
    mobilePanel.value = 'middle';
  } else if (selectedVersion.value) {
    populateEditForm(selectedVersion.value);
    isDirty.value = false;
  }
}

function confirmActivate(versionId: string) {
  pendingActionVersionId.value = versionId;
  showActivateModal.value = true;
}

async function doActivate() {
  if (!selectedSlug.value || !pendingActionVersionId.value) return;
  saving.value = true;
  try {
    await activateTemplateVersion(selectedSlug.value, pendingActionVersionId.value);
    toast.success(t('emailTemplates.versions.activateSuccess'));
    showActivateModal.value = false;
    await loadVersions(selectedSlug.value);
    if (selectedVersionId.value) {
      await loadVersion(selectedSlug.value, selectedVersionId.value);
    }
  } catch {
    toast.error(t('emailTemplates.versions.activateError'));
  } finally {
    saving.value = false;
  }
}

function confirmDelete(versionId: string) {
  pendingActionVersionId.value = versionId;
  showDeleteModal.value = true;
}

async function doDelete() {
  if (!selectedSlug.value || !pendingActionVersionId.value) return;
  saving.value = true;
  try {
    await deleteTemplateVersion(selectedSlug.value, pendingActionVersionId.value);
    toast.success(t('emailTemplates.versions.deleteSuccess'));
    showDeleteModal.value = false;
    if (selectedVersionId.value === pendingActionVersionId.value) {
      selectedVersionId.value = null;
      selectedVersion.value = null;
      isCreating.value = false;
      resetEditForm();
    }
    await loadVersions(selectedSlug.value);
  } catch {
    toast.error(t('emailTemplates.versions.deleteError'));
  } finally {
    saving.value = false;
  }
}

function copyVariable(variable: string) {
  navigator.clipboard.writeText(formatVariable(variable));
  toast.success(t('emailTemplates.variablesCopied'));
}

function openPreview() {
  showPreviewModal.value = true;
  previewWidth.value = 'desktop';
  previewLang.value = 'en';
}

const ALLOWED_FONT_PREFIXES = [
  'https://fonts.googleapis.com/',
  'https://fonts.gstatic.com/',
];

function isValidFontUrl(url: string): boolean {
  return ALLOWED_FONT_PREFIXES.some(prefix => url.startsWith(prefix));
}

const previewHtml = computed(() => {
  const lang = previewLang.value;
  const dir = lang === 'fa' ? 'rtl' : 'ltr';
  const fc = editForm.font_config[lang];
  const fontLink = fc?.url && isValidFontUrl(fc.url) ? `<link href="${fc.url}" rel="stylesheet">` : '';
  const fontFamily = fc?.family || 'sans-serif';

  return `<!DOCTYPE html>
<html lang="${lang}" dir="${dir}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
${fontLink}
<style>
body { font-family: '${fontFamily}', sans-serif; margin: 0; padding: 16px; }
${editForm.css_content}
</style>
</head>
<body>
${editForm.html_body}
</body>
</html>`;
});

function testFont(lang: string) {
  const fc = editForm.font_config[lang];
  if (!fc?.url) {
    toast.error(t('emailTemplates.editor.noFontUrl'));
    return;
  }
  if (!isValidFontUrl(fc.url)) {
    toast.error(t('emailTemplates.editor.invalidFontUrl'));
    return;
  }
  window.open(fc.url, '_blank', 'noopener');
}

// ─── Dirty tracking ───
watch(() => editForm.version_name, () => { isDirty.value = true; });
watch(() => editForm.html_body, () => { isDirty.value = true; });
watch(() => editForm.css_content, () => { isDirty.value = true; });
watch(() => editForm.font_config, () => { isDirty.value = true; }, { deep: true });

// ─── Computed ───
const selectedTemplateData = computed(() => {
  if (!selectedSlug.value) return null;
  return getTemplateBySlug(selectedSlug.value) || null;
});

const canCreateVersion = computed(() => versions.value.length < maxVersions.value);
const showEditor = computed(() => isCreating.value || selectedVersion.value !== null);

// ─── Init ───
onMounted(() => {
  loadTemplates();
});
</script>

<template>
  <div class="email-templates-page">
    <header class="page-header">
      <h1>{{ t('emailTemplates.title') }}</h1>
      <p class="subtitle">{{ t('emailTemplates.description') }}</p>
    </header>

    <!-- Mobile Navigation -->
    <div class="mobile-nav">
      <button
        v-if="mobilePanel !== 'left'"
        class="btn btn-ghost btn-sm"
        @click="mobilePanel = mobilePanel === 'right' ? 'middle' : 'left'"
      >
        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M19 12H5"/><polyline points="12 19 5 12 12 5"/></svg>
        {{ t('common.back') }}
      </button>
    </div>

    <div v-if="loading" class="loading">{{ t('common.loading') }}</div>

    <div v-else class="templates-layout">
      <!-- ═══ LEFT PANEL: Category & Template List ═══ -->
      <aside :class="['panel panel-left', { 'mobile-visible': mobilePanel === 'left' }]">
        <div class="panel-header">
          <h2>{{ t('emailTemplates.templates') }}</h2>
        </div>
        <div class="category-list">
          <div
            v-for="(cat, key) in categories"
            :key="key"
            class="category-group"
          >
            <button
              class="category-header"
              @click="toggleCategory(key as string)"
            >
              <span class="category-icon">{{ cat.icon }}</span>
              <span class="category-label">{{ cat.label }}</span>
              <svg
                :class="['chevron', { expanded: expandedCategories.has(key as string) }]"
                xmlns="http://www.w3.org/2000/svg" width="14" height="14"
                viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
              ><polyline points="6 9 12 15 18 9"/></svg>
            </button>
            <div v-if="expandedCategories.has(key as string)" class="category-items">
              <button
                v-for="slug in cat.slugs"
                :key="slug"
                :class="['template-item', { active: selectedSlug === slug }]"
                @click="selectSlug(slug)"
              >
                <div class="template-info">
                  <span class="template-name">{{ formatSlug(slug) }}</span>
                  <span class="template-desc" v-if="getTemplateBySlug(slug)">
                    {{ getTemplateBySlug(slug)!.description }}
                  </span>
                </div>
                <span
                  v-if="selectedSlug === slug && versions.length > 0"
                  class="version-badge"
                >{{ versions.length }}/{{ maxVersions }}</span>
              </button>
            </div>
          </div>
        </div>
      </aside>

      <!-- ═══ MIDDLE PANEL: Versions List ═══ -->
      <section :class="['panel panel-middle', { 'mobile-visible': mobilePanel === 'middle' }]">
        <div v-if="!selectedSlug" class="empty-state">
          <div class="empty-icon">
            <svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <rect x="2" y="4" width="20" height="16" rx="2" />
              <path d="M22 7l-10 7L2 7" />
            </svg>
          </div>
          <p>{{ t('emailTemplates.selectTemplate') }}</p>
        </div>

        <template v-else>
          <!-- Version Panel Header -->
          <div class="panel-header versions-header">
            <div class="versions-title-row">
              <h2>{{ formatSlug(selectedSlug) }}</h2>
            </div>
            <p v-if="selectedTemplateData" class="template-description">
              {{ selectedTemplateData.description }}
            </p>
            <!-- Available Variables -->
            <div v-if="selectedTemplateData?.variables" class="variables-chips">
              <span class="variables-label">{{ t('emailTemplates.variables') }}:</span>
              <button
                v-for="v in selectedTemplateData.variables.split(', ')"
                :key="v"
                class="variable-chip"
                @click="copyVariable(v)"
                :title="t('emailTemplates.copyVariables')"
              >{{ formatVariable(v) }}</button>
            </div>
          </div>

          <!-- Create New Version Button -->
          <div class="versions-actions" v-if="canEdit">
            <button
              class="btn btn-primary btn-sm"
              :disabled="!canCreateVersion"
              @click="startCreate"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
              {{ t('emailTemplates.versions.create') }}
            </button>
            <span v-if="!canCreateVersion" class="max-reached-hint">
              {{ t('emailTemplates.maxReached') }}
            </span>
          </div>

          <!-- Versions List -->
          <div v-if="loadingVersions" class="loading">{{ t('common.loading') }}</div>
          <div v-else-if="versions.length === 0" class="empty-state small">
            <p>{{ t('emailTemplates.versions.empty') }}</p>
          </div>
          <div v-else class="versions-list">
            <div
              v-for="ver in versions"
              :key="ver.id"
              :class="['version-card', { 'is-active': ver.is_active, selected: selectedVersionId === ver.id }]"
            >
              <div class="version-card-header">
                <span class="version-name">{{ ver.version_name }}</span>
                <span :class="['status-badge', ver.is_active ? 'active' : 'inactive']">
                  {{ ver.is_active ? t('emailTemplates.versions.active') : t('emailTemplates.versions.inactive') }}
                </span>
              </div>
              <div class="version-card-meta">
                <span class="version-date">{{ formatDate(ver.created_at) }}</span>
                <span class="version-fonts" v-if="ver.font_config">{{ fontSummary(ver.font_config) }}</span>
              </div>
              <div class="version-card-actions">
                <button class="btn btn-ghost btn-sm" @click="selectVersion(ver.id)">
                  {{ t('emailTemplates.versions.edit') }}
                </button>
                <button
                  v-if="!ver.is_active && canEdit"
                  class="btn btn-ghost btn-sm"
                  @click="confirmActivate(ver.id)"
                >{{ t('emailTemplates.versions.activate') }}</button>
                <button
                  v-if="!ver.is_active && canEdit"
                  class="btn btn-ghost btn-sm btn-danger-text"
                  @click="confirmDelete(ver.id)"
                >{{ t('emailTemplates.versions.delete') }}</button>
                <button class="btn btn-ghost btn-sm" @click="selectVersion(ver.id); openPreview()">
                  {{ t('emailTemplates.versions.preview') }}
                </button>
              </div>
            </div>
          </div>
        </template>
      </section>

      <!-- ═══ RIGHT PANEL: Editor ═══ -->
      <main :class="['panel panel-right', { 'mobile-visible': mobilePanel === 'right' }]">
        <div v-if="!showEditor" class="empty-state">
          <p>{{ t('emailTemplates.versions.selectOrCreate') }}</p>
        </div>

        <div v-else-if="loadingVersion" class="loading">{{ t('common.loading') }}</div>

        <template v-else>
          <!-- Editor Header -->
          <div class="panel-header editor-header">
            <h2>
              {{ isCreating
                ? t('emailTemplates.versions.createTitle')
                : t('emailTemplates.versions.editTitle') + ': ' + editForm.version_name }}
            </h2>
          </div>

          <div class="editor-body">
            <!-- Version Name -->
            <div class="form-group">
              <label for="version-name">{{ t('emailTemplates.editor.versionName') }}</label>
              <input
                id="version-name"
                type="text"
                class="form-input"
                v-model="editForm.version_name"
                :placeholder="t('emailTemplates.editor.versionNamePlaceholder')"
                :readonly="!canEdit"
              />
            </div>

            <!-- Tabs -->
            <div class="editor-tabs">
              <button
                :class="['tab', { active: editorTab === 'html' }]"
                @click="editorTab = 'html'"
              >{{ t('emailTemplates.editor.htmlBody') }}</button>
              <button
                :class="['tab', { active: editorTab === 'css' }]"
                @click="editorTab = 'css'"
              >{{ t('emailTemplates.editor.cssStyles') }}</button>
              <button
                :class="['tab', { active: editorTab === 'fonts' }]"
                @click="editorTab = 'fonts'"
              >{{ t('emailTemplates.editor.fonts') }}</button>
            </div>

            <!-- HTML Body Tab -->
            <div v-if="editorTab === 'html'" class="tab-panel">
              <div class="variables-chips editor-vars">
                <span class="variables-label">{{ t('emailTemplates.variables') }}:</span>
                <button
                  v-for="v in (selectedTemplateData?.variables || '').split(', ').filter(Boolean)"
                  :key="v"
                  class="variable-chip"
                  @click="copyVariable(v)"
                  :title="t('emailTemplates.editor.clickToCopy')"
                >{{ formatVariable(v) }}</button>
              </div>
              <textarea
                class="code-editor"
                v-model="editForm.html_body"
                :readonly="!canEdit"
                spellcheck="false"
                :placeholder="t('emailTemplates.editor.htmlPlaceholder')"
              />
            </div>

            <!-- CSS Styles Tab -->
            <div v-if="editorTab === 'css'" class="tab-panel">
              <div class="css-note">
                <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>
                {{ t('emailTemplates.editor.cssNote') }}
              </div>
              <textarea
                class="code-editor"
                v-model="editForm.css_content"
                :readonly="!canEdit"
                spellcheck="false"
                :placeholder="t('emailTemplates.editor.cssPlaceholder')"
              />
            </div>

            <!-- Fonts Tab -->
            <div v-if="editorTab === 'fonts'" class="tab-panel fonts-panel">
              <div v-for="lang in ['en', 'fa']" :key="lang" class="font-lang-section">
                <h4 class="font-lang-title">{{ lang === 'en' ? 'English' : 'Farsi' }} ({{ lang }})</h4>
                <div class="form-group">
                  <label>{{ t('emailTemplates.editor.fontFamily') }}</label>
                  <input
                    type="text"
                    class="form-input"
                    v-model="editForm.font_config[lang].family"
                    :readonly="!canEdit"
                  />
                </div>
                <div class="form-group">
                  <label>{{ t('emailTemplates.editor.fontWeight') }}</label>
                  <input
                    type="text"
                    class="form-input"
                    v-model="editForm.font_config[lang].weight"
                    :readonly="!canEdit"
                    placeholder="400;600;700"
                  />
                </div>
                <div class="form-group">
                  <label>{{ t('emailTemplates.editor.fontUrl') }}</label>
                  <div class="font-url-row">
                    <input
                      type="text"
                      class="form-input"
                      v-model="editForm.font_config[lang].url"
                      :readonly="!canEdit"
                    />
                    <button class="btn btn-outline btn-sm" @click="testFont(lang)">
                      {{ t('emailTemplates.editor.testFont') }}
                    </button>
                  </div>
                  <span
                    v-if="editForm.font_config[lang].url && !isValidFontUrl(editForm.font_config[lang].url)"
                    class="field-error"
                  >{{ t('emailTemplates.editor.invalidFontUrl') }}</span>
                </div>
              </div>
            </div>
          </div>

          <!-- Editor Footer -->
          <div class="editor-footer" v-if="canEdit">
            <div class="footer-left">
              <label v-if="isCreating" class="checkbox-label">
                <input type="checkbox" v-model="setActiveOnCreate" />
                {{ t('emailTemplates.editor.setActiveImmediately') }}
              </label>
            </div>
            <div class="footer-right">
              <button class="btn btn-secondary btn-sm" @click="cancelEdit">
                {{ t('emailTemplates.editor.cancel') }}
              </button>
              <button class="btn btn-outline btn-sm" @click="openPreview">
                {{ t('emailTemplates.versions.preview') }}
              </button>
              <button
                class="btn btn-primary btn-sm"
                @click="saveVersion"
                :disabled="saving || !editForm.version_name.trim()"
              >
                {{ saving ? t('common.saving') : t('emailTemplates.editor.save') }}
              </button>
            </div>
          </div>
        </template>
      </main>
    </div>

    <!-- ═══ MODALS ═══ -->

    <!-- Delete Confirmation Modal -->
    <Teleport to="body">
      <div v-if="showDeleteModal" class="modal-overlay" @click.self="showDeleteModal = false">
        <div class="modal">
          <h3>{{ t('emailTemplates.versions.delete') }}</h3>
          <p>{{ t('emailTemplates.deleteConfirm') }}</p>
          <div class="modal-actions">
            <button class="btn btn-secondary" @click="showDeleteModal = false">
              {{ t('common.cancel') }}
            </button>
            <button class="btn btn-danger" @click="doDelete" :disabled="saving">
              {{ t('emailTemplates.versions.delete') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Activate Confirmation Modal -->
    <Teleport to="body">
      <div v-if="showActivateModal" class="modal-overlay" @click.self="showActivateModal = false">
        <div class="modal">
          <h3>{{ t('emailTemplates.versions.activate') }}</h3>
          <p>{{ t('emailTemplates.activateConfirm') }}</p>
          <div class="modal-actions">
            <button class="btn btn-secondary" @click="showActivateModal = false">
              {{ t('common.cancel') }}
            </button>
            <button class="btn btn-primary" @click="doActivate" :disabled="saving">
              {{ t('emailTemplates.versions.activate') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Preview Modal -->
    <Teleport to="body">
      <div v-if="showPreviewModal" class="modal-overlay preview-overlay" @click.self="showPreviewModal = false">
        <div class="modal preview-modal">
          <div class="preview-modal-header">
            <h3>{{ t('emailTemplates.versions.preview') }}</h3>
            <div class="preview-controls">
              <div class="preview-toggle">
                <button
                  :class="['toggle-btn', { active: previewWidth === 'desktop' }]"
                  @click="previewWidth = 'desktop'"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
                  Desktop
                </button>
                <button
                  :class="['toggle-btn', { active: previewWidth === 'mobile' }]"
                  @click="previewWidth = 'mobile'"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="5" y="2" width="14" height="20" rx="2"/><line x1="12" y1="18" x2="12.01" y2="18"/></svg>
                  Mobile
                </button>
              </div>
              <div class="preview-toggle">
                <button
                  :class="['toggle-btn', { active: previewLang === 'en' }]"
                  @click="previewLang = 'en'"
                >EN</button>
                <button
                  :class="['toggle-btn', { active: previewLang === 'fa' }]"
                  @click="previewLang = 'fa'"
                >FA</button>
              </div>
              <button class="btn btn-ghost btn-sm" @click="showPreviewModal = false">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
              </button>
            </div>
          </div>
          <div class="preview-frame-container">
            <iframe
              class="preview-frame"
              :style="{ width: previewWidth === 'desktop' ? '600px' : '375px' }"
              :srcdoc="previewHtml"
              sandbox="allow-same-origin"
              :title="t('adminEmail.previewTitle')"
            />
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.email-templates-page {
  padding: var(--spacing-lg);
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.page-header {
  margin-bottom: var(--spacing-lg);
  flex-shrink: 0;
}

.page-header h1 {
  font-size: var(--font-size-2xl);
  font-weight: 700;
  color: var(--color-text-primary);
  margin: 0;
}

.subtitle {
  color: var(--color-text-secondary);
  margin-top: var(--spacing-xs);
  font-size: var(--font-size-sm);
}

/* Mobile nav */
.mobile-nav {
  display: none;
  margin-bottom: var(--spacing-sm);
}

.loading {
  padding: var(--spacing-xl);
  text-align: center;
  color: var(--color-text-secondary);
}

/* ─── Three Panel Layout ─── */
.templates-layout {
  display: flex;
  gap: var(--spacing-md);
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.panel {
  background: var(--color-bg-secondary);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.panel-left {
  width: 280px;
  flex-shrink: 0;
}

.panel-middle {
  width: 300px;
  flex-shrink: 0;
}

.panel-right {
  flex: 1;
  min-width: 0;
}

.panel-header {
  padding: var(--spacing-md);
  border-bottom: 1px solid var(--color-border);
  flex-shrink: 0;
}

.panel-header h2 {
  font-size: var(--font-size-md);
  font-weight: 600;
  margin: 0;
  color: var(--color-text-primary);
}

/* ─── Left Panel: Categories ─── */
.category-list {
  flex: 1;
  overflow-y: auto;
  padding: var(--spacing-xs);
}

.category-group {
  margin-bottom: var(--spacing-xs);
}

.category-header {
  width: 100%;
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  border: none;
  background: transparent;
  cursor: pointer;
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-primary);
  transition: background-color 0.15s;
}

.category-header:hover {
  background: var(--color-bg-tertiary);
}

.category-icon {
  font-size: var(--font-size-md);
  flex-shrink: 0;
}

.category-label {
  flex: 1;
  text-align: left;
}

.chevron {
  transition: transform 0.2s;
  flex-shrink: 0;
  color: var(--color-text-muted);
}

.chevron.expanded {
  transform: rotate(180deg);
}

.category-items {
  padding-left: var(--spacing-sm);
}

.template-item {
  width: 100%;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--spacing-sm) var(--spacing-md);
  border: none;
  background: transparent;
  border-radius: var(--radius-md);
  cursor: pointer;
  text-align: left;
  transition: background-color 0.15s;
  margin-bottom: 1px;
  gap: var(--spacing-sm);
}

.template-item:hover {
  background: var(--color-bg-tertiary);
}

.template-item.active {
  background: var(--color-primary-light);
}

.template-info {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
  flex: 1;
}

.template-name {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.template-desc {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.version-badge {
  font-size: 10px;
  padding: 1px 6px;
  border-radius: var(--radius-full);
  background: var(--color-primary-light);
  color: var(--color-primary);
  font-weight: 600;
  flex-shrink: 0;
}

/* ─── Middle Panel: Versions ─── */
.versions-header {
  padding-bottom: var(--spacing-sm);
}

.versions-title-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.template-description {
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
  margin: var(--spacing-xs) 0 0;
}

.variables-chips {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  margin-top: var(--spacing-sm);
}

.variables-label {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  font-weight: 500;
}

.variable-chip {
  font-family: var(--font-mono);
  font-size: 10px;
  padding: 1px 5px;
  background: var(--color-bg-tertiary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  color: var(--color-text-primary);
  cursor: pointer;
  transition: all 0.15s;
}

.variable-chip:hover {
  background: var(--color-primary-light);
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.versions-actions {
  padding: var(--spacing-sm) var(--spacing-md);
  border-bottom: 1px solid var(--color-border);
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-shrink: 0;
}

.max-reached-hint {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

.versions-list {
  flex: 1;
  overflow-y: auto;
  padding: var(--spacing-sm);
}

.version-card {
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  margin-bottom: var(--spacing-sm);
  background: var(--color-bg-primary);
  transition: all 0.15s;
}

.version-card.is-active {
  border-left: 3px solid var(--color-success, #10B981);
  background: color-mix(in srgb, var(--color-success, #10B981) 5%, var(--color-bg-primary));
}

.version-card.selected {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 1px var(--color-primary);
}

.version-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 4px;
}

.version-name {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-primary);
}

.status-badge {
  font-size: 10px;
  padding: 1px 6px;
  border-radius: var(--radius-full);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.02em;
}

.status-badge.active {
  background: color-mix(in srgb, var(--color-success, #10B981) 15%, transparent);
  color: var(--color-success, #10B981);
}

.status-badge.inactive {
  background: var(--color-bg-tertiary);
  color: var(--color-text-muted);
}

.version-card-meta {
  display: flex;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-sm);
}

.version-date,
.version-fonts {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}

.version-card-actions {
  display: flex;
  gap: var(--spacing-xs);
  flex-wrap: wrap;
}

.btn-danger-text {
  color: var(--color-error) !important;
}

.btn-danger-text:hover {
  background: color-mix(in srgb, var(--color-error) 10%, transparent) !important;
}

/* ─── Right Panel: Editor ─── */
.editor-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.editor-body {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  padding: 0 var(--spacing-md);
}

.form-group {
  margin-bottom: var(--spacing-sm);
  flex-shrink: 0;
}

.form-group label {
  display: block;
  font-size: var(--font-size-xs);
  font-weight: 500;
  color: var(--color-text-secondary);
  margin-bottom: 4px;
}

.form-input {
  width: 100%;
  padding: var(--spacing-sm);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-bg-primary);
  color: var(--color-text-primary);
  font-size: var(--font-size-sm);
}

.form-input:focus {
  outline: none;
  border-color: var(--color-primary);
}

.editor-tabs {
  display: flex;
  border-bottom: 1px solid var(--color-border);
  margin-bottom: var(--spacing-sm);
  flex-shrink: 0;
}

.tab {
  padding: var(--spacing-sm) var(--spacing-md);
  border: none;
  background: transparent;
  cursor: pointer;
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-secondary);
  border-bottom: 2px solid transparent;
  transition: all 0.15s;
}

.tab:hover {
  color: var(--color-text-primary);
}

.tab.active {
  color: var(--color-primary);
  border-bottom-color: var(--color-primary);
}

.tab-panel {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  min-height: 0;
}

.editor-vars {
  margin-bottom: var(--spacing-sm);
  flex-shrink: 0;
}

.css-note {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm);
  background: var(--color-bg-tertiary);
  border-radius: var(--radius-md);
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-sm);
  flex-shrink: 0;
}

.code-editor {
  flex: 1;
  width: 100%;
  font-family: var(--font-mono);
  font-size: var(--font-size-sm);
  line-height: 1.5;
  padding: var(--spacing-sm);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-bg-primary);
  color: var(--color-text-primary);
  resize: none;
  min-height: 200px;
}

.code-editor:focus {
  outline: none;
  border-color: var(--color-primary);
}

/* Fonts panel */
.fonts-panel {
  overflow-y: auto;
  gap: var(--spacing-md);
}

.font-lang-section {
  padding: var(--spacing-md);
  background: var(--color-bg-tertiary);
  border-radius: var(--radius-md);
  margin-bottom: var(--spacing-md);
}

.font-lang-title {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0 0 var(--spacing-sm) 0;
}

.font-url-row {
  display: flex;
  gap: var(--spacing-sm);
}

.font-url-row .form-input {
  flex: 1;
}

/* Editor footer */
.editor-footer {
  padding: var(--spacing-sm) var(--spacing-md);
  border-top: 1px solid var(--color-border);
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-shrink: 0;
}

.footer-left {
  display: flex;
  align-items: center;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  cursor: pointer;
}

.checkbox-label input[type="checkbox"] {
  accent-color: var(--color-primary);
}

.footer-right {
  display: flex;
  gap: var(--spacing-sm);
}

/* ─── Empty States ─── */
.empty-state {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: var(--color-text-muted);
  padding: var(--spacing-xl);
}

.empty-state.small {
  padding: var(--spacing-lg);
}

.empty-state p {
  font-size: var(--font-size-sm);
  text-align: center;
}

.empty-icon {
  margin-bottom: var(--spacing-md);
  opacity: 0.4;
}

/* ─── Buttons ─── */
.btn {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s;
  border: 1px solid transparent;
  white-space: nowrap;
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-primary {
  background: var(--color-primary);
  color: white;
}

.btn-primary:hover:not(:disabled) {
  background: var(--color-primary-dark, #4f46e5);
}

.btn-secondary {
  background: var(--color-bg-tertiary);
  color: var(--color-text-primary);
  border-color: var(--color-border);
}

.btn-secondary:hover:not(:disabled) {
  background: var(--color-bg-primary);
}

.btn-outline {
  background: transparent;
  color: var(--color-text-secondary);
  border-color: var(--color-border);
}

.btn-outline:hover:not(:disabled) {
  background: var(--color-bg-tertiary);
}

.btn-danger {
  background: var(--color-error);
  color: white;
}

.btn-danger:hover:not(:disabled) {
  background: var(--color-error-dark, #b91c1c);
}

.btn-ghost {
  background: transparent;
  color: var(--color-text-secondary);
}

.btn-ghost:hover:not(:disabled) {
  background: var(--color-bg-tertiary);
}

.btn-sm {
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
}

/* ─── Modals ─── */
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: var(--z-modal, 300);
}

.modal {
  background: var(--color-bg-secondary);
  border-radius: var(--radius-lg);
  padding: var(--spacing-lg);
  max-width: 420px;
  width: 90%;
  box-shadow: var(--shadow-lg);
}

.modal h3 {
  margin: 0 0 var(--spacing-sm) 0;
  font-size: var(--font-size-lg);
  color: var(--color-text-primary);
}

.modal p {
  color: var(--color-text-secondary);
  margin-bottom: var(--spacing-lg);
  font-size: var(--font-size-sm);
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-sm);
}

/* Preview Modal */
.preview-overlay {
  z-index: var(--z-modal, 300);
}

.preview-modal {
  max-width: 90vw;
  width: 700px;
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  padding: 0;
}

.preview-modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--spacing-md) var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
  flex-shrink: 0;
}

.preview-modal-header h3 {
  margin: 0;
  font-size: var(--font-size-md);
}

.preview-controls {
  display: flex;
  gap: var(--spacing-md);
  align-items: center;
}

.preview-toggle {
  display: flex;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  overflow: hidden;
}

.toggle-btn {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  border: none;
  background: transparent;
  cursor: pointer;
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  transition: all 0.15s;
}

.toggle-btn.active {
  background: var(--color-primary-light);
  color: var(--color-primary);
}

.toggle-btn:hover:not(.active) {
  background: var(--color-bg-tertiary);
}

.preview-frame-container {
  flex: 1;
  overflow: auto;
  display: flex;
  justify-content: center;
  padding: var(--spacing-lg);
  background: var(--color-bg-tertiary);
}

.preview-frame {
  height: 600px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: white;
  transition: width 0.3s ease;
}

/* ─── RTL Support ─── */
[dir="rtl"] .templates-layout {
  flex-direction: row-reverse;
}

[dir="rtl"] .template-item {
  text-align: right;
}

[dir="rtl"] .category-label {
  text-align: right;
}

[dir="rtl"] .version-card.is-active {
  border-left: none;
  border-right: 3px solid var(--color-success, #10B981);
}

[dir="rtl"] .footer-right {
  flex-direction: row-reverse;
}

/* ─── Responsive Design ─── */

/* Tablet: 768px - 1200px */
@media (max-width: 1200px) {
  .panel-left {
    width: 240px;
  }

  .panel-middle {
    width: 260px;
  }
}

/* Tablet small: < 1024px - collapse left panel */
@media (max-width: 1024px) {
  .templates-layout {
    flex-direction: column;
  }

  .panel-left {
    width: 100%;
    max-height: 250px;
  }

  .panel-middle {
    width: 100%;
    max-height: 300px;
  }

  .panel-right {
    width: 100%;
  }
}

/* Mobile: < 768px */
@media (max-width: 768px) {
  .email-templates-page {
    padding: var(--spacing-md);
  }

  .mobile-nav {
    display: flex;
  }

  .templates-layout {
    flex-direction: row;
  }

  .panel {
    display: none;
    width: 100%;
    flex: 1;
  }

  .panel.mobile-visible {
    display: flex;
  }

  .panel-left {
    max-height: none;
  }

  .panel-middle {
    max-height: none;
  }
}

/* common.saving isn't defined — fallback text */

.field-error {
  font-size: var(--font-size-xs);
  color: var(--color-error, #ef4444);
  margin-top: var(--spacing-xs);
}
</style>
