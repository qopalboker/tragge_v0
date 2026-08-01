<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { t } from '@/i18n';
import { useAuthStore } from '@/stores/auth';
import { useI18nStore } from '@/stores/i18n';
import { useReferralStore } from '@/stores/referral';
import { useToast } from '@/composables/useToast';
import { oauthApi } from '@/api/oauth';
import { getErrorMessage } from '@/utils/errorHandler';
import { validateRedirectPath, getTradeRedirect } from '@/utils/redirectValidation';
import { redirectToTrade } from '@/utils/tradeRedirect';
import { COUNTRY_DATA } from '@/constants/countries';
import { ARCAPTCHA_SITE_KEY } from '@/config/captcha';
import VerificationFlow from '@/components/auth/VerificationFlow.vue';

const router = useRouter();
const route = useRoute();
const authStore = useAuthStore();
const i18nStore = useI18nStore();
const referralStore = useReferralStore();
const toast = useToast();

// State
const mounted = ref(false);
const isSignup = ref(false);

// Verification modal state
const showVerification = ref(false);
const verificationMethods = ref<string[]>([]);
const verificationMaskedEmail = ref('');
const verificationMaskedPhone = ref('');
const showPass = ref(false);
const phoneDropdownOpen = ref(false);
const phoneSearch = ref('');
const googleLoading = ref(false);
const captchaLoading = ref(false);
const captchaWidgetId = ref<string | null>(null);
let captchaResolve: ((token: string) => void) | null = null;
let captchaReject: ((err: Error) => void) | null = null;

// Form data
const formData = ref({
  firstName: '',
  lastName: '',
  country: '',
  email: '',
  phone: '',
  phoneCountry: 'IR',
  password: '',
  confirmPass: '',
  username: '',
  agreeTerms: false,
  ageConfirm: false,
});

// Touched fields for validation
const touched = ref<Record<string, boolean>>({});

// Errors
const errors = ref<Record<string, string>>({});

// Computed
const isRTL = computed(() => i18nStore.locale === 'fa');
const lang = computed(() => i18nStore.locale);

const selectedPhoneCountry = computed(() => {
  return COUNTRY_DATA.find(c => c.code === formData.value.phoneCountry) || COUNTRY_DATA.find(c => c.code === 'IR')!;
});

const filteredCountries = computed(() => {
  if (!phoneSearch.value) return COUNTRY_DATA;
  const search = phoneSearch.value.toLowerCase();
  return COUNTRY_DATA.filter(c =>
    c.name.toLowerCase().includes(search) ||
    c.dial.includes(search) ||
    c.code.toLowerCase().includes(search)
  );
});

const isFormReady = computed(() => {
  if (isSignup.value) {
    return (
      formData.value.firstName.trim() &&
      formData.value.lastName.trim() &&
      formData.value.country &&
      formData.value.username.trim() &&
      /^[a-zA-Z0-9]+$/.test(formData.value.username) &&
      /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(formData.value.email) &&
      formData.value.password.length >= 10 &&
      formData.value.confirmPass === formData.value.password &&
      formData.value.agreeTerms &&
      formData.value.ageConfirm
    );
  }
  return (
    /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(formData.value.email) &&
    formData.value.password.length >= 10
  );
});


// Helper functions
const lettersOnly = (v: string) => v.replace(/[^a-zA-Z\u0600-\u06FF\u0750-\u077F\s]/g, '');
const alphanumOnly = (v: string) => v.replace(/[^a-zA-Z0-9]/g, '');
const digitsOnly = (v: string) => v.replace(/[^0-9]/g, '');

// Validation
function validate() {
  const e: Record<string, string> = {};
  const data = formData.value;

  if (isSignup.value) {
    if (!data.firstName.trim()) e.firstName = t('auth.errorRequired');
    if (!data.lastName.trim()) e.lastName = t('auth.errorRequired');
    if (!data.country) e.country = t('auth.errorRequired');
    if (!data.username.trim()) e.username = t('auth.errorRequired');
    else if (!/^[a-zA-Z0-9]+$/.test(data.username)) e.username = t('auth.errorUsernameFormat');
    if (data.password && data.confirmPass && data.password !== data.confirmPass) e.confirmPass = t('auth.errorPasswordMatch');
    if (!data.confirmPass) e.confirmPass = t('auth.errorRequired');
  }
  if (!data.email.trim()) e.email = t('auth.errorRequired');
  else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(data.email)) e.email = t('auth.errorInvalidEmail');
  if (!data.password) e.password = t('auth.errorRequired');
  else if (data.password.length < 10) e.password = t('auth.errorPasswordLength');

  errors.value = e;
}

// Watch for validation
watch([formData, isSignup, touched], () => {
  if (Object.keys(touched.value).length > 0) {
    validate();
  }
}, { deep: true });

// Event handlers
function setField(key: string, value: any) {
  (formData.value as any)[key] = value;
}

function blur(key: string) {
  touched.value[key] = true;
}

function hasError(field: string) {
  return touched.value[field] && errors.value[field];
}

function onFirstName(e: Event) {
  setField('firstName', lettersOnly((e.target as HTMLInputElement).value));
}

function onLastName(e: Event) {
  setField('lastName', lettersOnly((e.target as HTMLInputElement).value));
}

function onUsername(e: Event) {
  setField('username', alphanumOnly((e.target as HTMLInputElement).value));
}

function onPhone(e: Event) {
  let v = digitsOnly((e.target as HTMLInputElement).value);
  if (v.startsWith('0')) v = v.slice(1);
  if (v.length > 10) v = v.slice(0, 10);
  setField('phone', v);
}

function selectPhoneCountry(code: string) {
  setField('phoneCountry', code);
  phoneDropdownOpen.value = false;
  phoneSearch.value = '';
}

async function handleSubmit() {
  // Touch all fields
  const fields = isSignup.value
    ? ['firstName', 'lastName', 'country', 'email', 'phone', 'password', 'confirmPass', 'username', 'agreeTerms', 'ageConfirm']
    : ['email', 'password'];
  fields.forEach(f => touched.value[f] = true);

  validate();

  if (Object.keys(errors.value).length > 0 || !isFormReady.value) {
    return;
  }

  if (isSignup.value) {
    // Execute invisible ARCaptcha before registration
    captchaLoading.value = true;
    let captchaToken: string;
    try {
      captchaToken = await executeCaptcha();
      if (!captchaToken) {
        toast.error(t('auth.captchaFailed'));
        return;
      }
    } catch {
      toast.error(t('auth.captchaFailed'));
      return;
    } finally {
      captchaLoading.value = false;
    }

    // Registration — send profile data + terms/age confirmation alongside auth
    const phoneCountryData = COUNTRY_DATA.find(c => c.code === formData.value.phoneCountry);
    const fullPhone = formData.value.phone
      ? `${phoneCountryData?.dial || '+98'}${formData.value.phone}`
      : undefined;

    const success = await authStore.register(
      formData.value.email,
      formData.value.password,
      {
        username: formData.value.username || undefined,
        display_name: `${formData.value.firstName} ${formData.value.lastName}`.trim() || undefined,
        country: formData.value.country,
        phone: fullPhone,
      },
      formData.value.agreeTerms,
      formData.value.ageConfirm,
      referralStore.referralCode || undefined,
      captchaToken,
    );
    if (success) {
      toast.success(t('auth.loginSuccess'));
      referralStore.clearReferralCode();
      // If email not verified, show verification modal
      if (!authStore.user?.email_verified) {
        openVerificationModal();
      } else {
        handleAuthRedirect();
      }
    } else {
      toast.error(authStore.error || t('auth.loginError'));
    }
  } else {
    // Login
    const success = await authStore.login(formData.value.email, formData.value.password);
    if (success) {
      toast.success(t('auth.loginSuccess'));
      referralStore.clearReferralCode();
      // If email not verified, show verification modal
      if (!authStore.user?.email_verified) {
        openVerificationModal();
      } else {
        handleAuthRedirect();
      }
    } else {
      toast.error(authStore.error || t('auth.loginError'));
    }
  }
}

function openVerificationModal() {
  const authResp = authStore.lastAuthResponse;
  if (authResp?.available_methods?.length) {
    // Use server-provided verification data
    verificationMethods.value = authResp.available_methods;
    verificationMaskedEmail.value = authResp.masked_email || '';
    verificationMaskedPhone.value = authResp.masked_phone || '';
  } else {
    // Fallback: derive from user data
    const methods: string[] = [];
    if (authStore.user?.email) methods.push('email');
    if (authStore.user?.phone) methods.push('sms');
    if (methods.length === 0) methods.push('email');
    verificationMethods.value = methods;
    verificationMaskedEmail.value = authStore.user?.email
      ? maskEmailClient(authStore.user.email) : '';
    verificationMaskedPhone.value = authStore.user?.phone
      ? maskPhoneClient(authStore.user.phone) : '';
  }
  showVerification.value = true;
}

function maskEmailClient(email: string): string {
  const parts = email.split('@');
  if (parts.length !== 2) return '***';
  const local = parts[0];
  if (local.length <= 1) return local + '***@' + parts[1];
  return local[0] + '***@' + parts[1];
}

function maskPhoneClient(phone: string): string {
  if (phone.length <= 4) return '***';
  if (phone.length <= 7) return phone.slice(0, 2) + '***' + phone.slice(-2);
  return phone.slice(0, 4) + '***' + phone.slice(-2);
}

async function onVerified() {
  showVerification.value = false;
  // Refresh user data
  await authStore.fetchUser();
  handleAuthRedirect();
}

function onVerificationClose() {
  showVerification.value = false;
  // Mark as dismissed so UserLayout doesn't re-show the modal
  sessionStorage.setItem('verification_dismissed', '1');
  // User can still continue unverified — redirect to dashboard
  handleAuthRedirect();
}

function handleAuthRedirect() {
  const redirect = validateRedirectPath(route.query.redirect as string);

  // Extract contest ID from trade redirect URLs (relative or absolute)
  // Patterns: /trade/<contestId>, https://...-5174.app.github.dev/trade/<contestId>, http://localhost:5174/trade/<contestId>
  const tradeMatch = redirect.match(/\/trade\/([0-9a-f-]{36})/);
  if (tradeMatch) {
    // Use redirectToTrade which handles cross-origin ticket-based auth
    redirectToTrade(tradeMatch[1]);
    return;
  }

  const tradeUrl = getTradeRedirect(redirect);
  if (tradeUrl) {
    window.location.href = tradeUrl;
    return;
  }

  router.push(redirect);
}

function toggleMode() {
  isSignup.value = !isSignup.value;
  captchaWidgetId.value = null;
  formData.value = {
    firstName: '',
    lastName: '',
    country: '',
    email: '',
    phone: '',
    phoneCountry: 'IR',
    password: '',
    confirmPass: '',
    username: '',
    agreeTerms: false,
    ageConfirm: false,
  };
  errors.value = {};
  touched.value = {};
  showPass.value = false;
  authStore.clearError();
}

function toggleLanguage() {
  i18nStore.toggleLocale();
}

function goToForgotPassword() {
  router.push('/user/forgot-password');
}

async function handleGoogleLogin() {
  if (googleLoading.value) return;

  googleLoading.value = true;
  authStore.clearError();

  try {
    // Store the validated redirect URL before redirecting to Google
    const redirect = validateRedirectPath(route.query.redirect as string);
    sessionStorage.setItem('oauth_redirect', redirect);

    // Get the Google OAuth URL and state from the backend
    const response = await oauthApi.getGoogleAuthUrl();

    if (response.auth_url && response.state) {
      // Store the backend's state for CSRF validation during callback
      sessionStorage.setItem('oauth_state', response.state);
      sessionStorage.setItem('oauth_state_timestamp', Date.now().toString());
      
      // Redirect to Google OAuth page
      window.location.href = response.auth_url;
    } else {
      throw new Error('No auth URL received');
    }
  } catch (err: unknown) {
    const message = getErrorMessage(err, t('auth.googleLoginError'));
    toast.error(message);
    googleLoading.value = false;
  }
  // Note: googleLoading stays true during redirect to prevent double-clicks
}

// Click outside handler for phone dropdown
function handleClickOutside(e: MouseEvent) {
  const target = e.target as HTMLElement;
  if (!target.closest('.phone-dropdown-wrapper')) {
    phoneDropdownOpen.value = false;
    phoneSearch.value = '';
  }
}

function executeCaptcha(): Promise<string> {
  return new Promise((resolve, reject) => {
    const win = window as any;
    if (!win.arcaptcha) {
      reject(new Error('ARCaptcha not loaded'));
      return;
    }

    // Timeout: reject after 30 seconds if no response from widget
    const timeout = setTimeout(() => {
      captchaResolve = null;
      captchaReject = null;
      reject(new Error('Captcha timeout'));
    }, 30000);

    captchaResolve = (token: string) => {
      clearTimeout(timeout);
      resolve(token);
    };
    captchaReject = (err: Error) => {
      clearTimeout(timeout);
      reject(err);
    };

    // Render invisible widget if not already rendered
    if (captchaWidgetId.value === null) {
      const container = document.getElementById('arcaptcha-container');
      captchaWidgetId.value = win.arcaptcha.render(container, {
        'site-key': ARCAPTCHA_SITE_KEY,
        size: 'invisible',
        callback: (token: string) => {
          if (captchaResolve) captchaResolve(token);
        },
        error_callback: () => {
          if (captchaReject) captchaReject(new Error('Captcha challenge failed'));
        },
      });
    } else {
      win.arcaptcha.reset(captchaWidgetId.value);
    }

    win.arcaptcha.execute(captchaWidgetId.value);
  });
}

// Lifecycle
onMounted(() => {
  setTimeout(() => {
    mounted.value = true;
  }, 100);
  document.addEventListener('mousedown', handleClickOutside);

  // Load ARCaptcha script
  if (!document.querySelector('script[src*="arcaptcha"]')) {
    const script = document.createElement('script');
    script.src = 'https://widget.arcaptcha.co/1/api.js';
    script.async = true;
    script.defer = true;
    document.head.appendChild(script);
  }

  // Check for referral code
  const refFromUrl = route.query.ref as string;
  if (refFromUrl) {
    referralStore.setReferralCode(refFromUrl, true);
    isSignup.value = true;
  } else if (referralStore.referralCode && referralStore.fromUrl) {
    isSignup.value = true;
  }
});

onUnmounted(() => {
  document.removeEventListener('mousedown', handleClickOutside);
  captchaResolve = null;
  captchaReject = null;
  captchaWidgetId.value = null;
});
</script>

<template>
  <div
    class="login-root"
    :style="{ direction: isRTL ? 'rtl' : 'ltr' }"
  >
    <!-- Background -->
    <div class="bg-image" />
    <!-- Dark overlay -->
    <div class="bg-overlay" />
    <!-- Grid overlay -->
    <div class="grid-overlay" />

    <!-- Card -->
    <div
      class="login-card"
      :class="{ mounted, signup: isSignup }"
    >
      <div class="card-inner" :class="{ signup: isSignup }">
        <!-- Accent line -->
        <div class="accent-line" />

        <!-- Language toggle -->
        <div class="lang-row">
          <div class="lang-buttons">
            <button
              type="button"
              :class="['lang-btn', { active: lang === 'en' }]"
              @click="() => i18nStore.locale !== 'en' && toggleLanguage()"
            >EN</button>
            <button
              type="button"
              :class="['lang-btn', { active: lang === 'fa' }]"
              @click="() => i18nStore.locale !== 'fa' && toggleLanguage()"
            >FA</button>
          </div>
        </div>

        <!-- Brand header -->
        <div class="brand-header" :class="{ signup: isSignup }">
          <div class="logo-wrapper">
            <div class="logo-box">
              <div class="logo-dot" />
            </div>
          </div>
          <div class="brand-title">{{ isSignup ? t('auth.createAccount') : t('auth.welcomeBack') }}</div>
          <div class="brand-sub">{{ isSignup ? t('auth.signupSubtitle') : t('auth.loginSubtitle') }}</div>
        </div>

        <!-- Form -->
        <form @submit.prevent="handleSubmit" class="auth-form" :class="{ signup: isSignup }">
          <!-- Signup fields -->
          <template v-if="isSignup">
            <!-- Row: First / Last name -->
            <div class="form-row">
              <div class="form-field half">
                <label class="field-label">{{ t('auth.firstName') }}</label>
                <input
                  type="text"
                  class="field-input"
                  :class="{ error: hasError('firstName') }"
                  :placeholder="t('auth.firstNamePlaceholder')"
                  :value="formData.firstName"
                  @input="onFirstName"
                  @blur="() => blur('firstName')"
                />
                <span v-if="hasError('firstName')" class="field-error">{{ errors.firstName }}</span>
              </div>
              <div class="form-field half">
                <label class="field-label">{{ t('auth.lastName') }}</label>
                <input
                  type="text"
                  class="field-input"
                  :class="{ error: hasError('lastName') }"
                  :placeholder="t('auth.lastNamePlaceholder')"
                  :value="formData.lastName"
                  @input="onLastName"
                  @blur="() => blur('lastName')"
                />
                <span v-if="hasError('lastName')" class="field-error">{{ errors.lastName }}</span>
              </div>
            </div>

            <!-- Row: Country / Username -->
            <div class="form-row">
              <div class="form-field half">
                <label class="field-label">{{ t('auth.country') }}</label>
                <select
                  class="field-select"
                  :class="{ error: hasError('country') }"
                  :value="formData.country"
                  @change="(e) => setField('country', (e.target as HTMLSelectElement).value)"
                  @blur="() => blur('country')"
                >
                  <option value="">{{ t('auth.selectCountry') }}</option>
                  <option v-for="c in COUNTRY_DATA" :key="c.code" :value="c.code">
                    {{ c.flag }} {{ c.name }}
                  </option>
                </select>
                <span v-if="hasError('country')" class="field-error">{{ errors.country }}</span>
              </div>
              <div class="form-field half">
                <label class="field-label">{{ t('auth.username') }}</label>
                <input
                  type="text"
                  class="field-input ltr"
                  :class="{ error: hasError('username') }"
                  :placeholder="t('auth.usernamePlaceholder')"
                  :value="formData.username"
                  @input="onUsername"
                  @blur="() => blur('username')"
                />
                <span v-if="hasError('username')" class="field-error">{{ errors.username }}</span>
              </div>
            </div>

            <!-- Row: Email / Phone -->
            <div class="form-row">
              <div class="form-field half">
                <label class="field-label">{{ t('auth.email') }}</label>
                <input
                  type="email"
                  class="field-input ltr"
                  :class="{ error: hasError('email') }"
                  :placeholder="t('auth.emailInputPlaceholder')"
                  :value="formData.email"
                  @input="(e) => setField('email', (e.target as HTMLInputElement).value)"
                  @blur="() => blur('email')"
                />
                <span v-if="hasError('email')" class="field-error">{{ errors.email }}</span>
              </div>
              <div class="form-field half phone-dropdown-wrapper">
                <label class="field-label">{{ t('auth.phone') }}</label>
                <div class="phone-input-row">
                  <button
                    type="button"
                    class="phone-code-btn"
                    :class="{ error: hasError('phone') }"
                    @click="phoneDropdownOpen = !phoneDropdownOpen"
                  >
                    <span class="phone-flag">{{ selectedPhoneCountry.flag }}</span>
                    <span class="phone-dial">{{ selectedPhoneCountry.dial }}</span>
                    <span class="phone-arrow">&#9660;</span>
                  </button>
                  <input
                    type="text"
                    class="field-input phone-number ltr"
                    :class="{ error: hasError('phone') }"
                    :placeholder="t('auth.phonePlaceholder')"
                    :value="formData.phone"
                    @input="onPhone"
                    @blur="() => blur('phone')"
                  />
                  <!-- Phone dropdown -->
                  <div v-if="phoneDropdownOpen" class="phone-dropdown">
                    <div class="phone-search-wrapper">
                      <input
                        type="text"
                        class="phone-search"
                        placeholder="Search..."
                        v-model="phoneSearch"
                        @keydown.escape="phoneDropdownOpen = false"
                      />
                    </div>
                    <div class="phone-list">
                      <div v-if="filteredCountries.length === 0" class="phone-no-results">
                        No results
                      </div>
                      <div
                        v-for="c in filteredCountries"
                        :key="c.code"
                        class="phone-option"
                        :class="{ selected: c.code === formData.phoneCountry }"
                        @click="selectPhoneCountry(c.code)"
                      >
                        <span class="phone-option-flag">{{ c.flag }}</span>
                        <span class="phone-option-name">{{ c.name }}</span>
                        <span class="phone-option-dial">{{ c.dial }}</span>
                      </div>
                    </div>
                  </div>
                </div>
                <span v-if="hasError('phone')" class="field-error">{{ errors.phone }}</span>
              </div>
            </div>

            <!-- Row: Password / Confirm -->
            <div class="form-row">
              <div class="form-field half">
                <label class="field-label">{{ t('auth.password') }}</label>
                <div class="password-wrapper">
                  <input
                    :type="showPass ? 'text' : 'password'"
                    class="field-input ltr"
                    :class="{ error: hasError('password') }"
                    :placeholder="t('auth.passwordPlaceholder')"
                    :value="formData.password"
                    @input="(e) => setField('password', (e.target as HTMLInputElement).value)"
                    @blur="() => blur('password')"
                  />
                  <button type="button" class="eye-btn" @click="showPass = !showPass">
                    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                      <template v-if="showPass">
                        <path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94"/>
                        <path d="M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19"/>
                        <line x1="1" y1="1" x2="23" y2="23"/>
                      </template>
                      <template v-else>
                        <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
                        <circle cx="12" cy="12" r="3"/>
                      </template>
                    </svg>
                  </button>
                </div>
                <span v-if="hasError('password')" class="field-error">{{ errors.password }}</span>
              </div>
              <div class="form-field half">
                <label class="field-label">{{ t('auth.confirmPassword') }}</label>
                <input
                  type="password"
                  class="field-input ltr"
                  :class="{ error: hasError('confirmPass') }"
                  :placeholder="t('auth.confirmPasswordPlaceholder')"
                  :value="formData.confirmPass"
                  @input="(e) => setField('confirmPass', (e.target as HTMLInputElement).value)"
                  @blur="() => blur('confirmPass')"
                />
                <span v-if="hasError('confirmPass')" class="field-error">{{ errors.confirmPass }}</span>
              </div>
            </div>

            <!-- Checkboxes -->
            <div class="checkbox-group">
              <label class="checkbox-label" @click="setField('agreeTerms', !formData.agreeTerms)">
                <div class="checkbox-box" :class="{ checked: formData.agreeTerms }">
                  <svg v-if="formData.agreeTerms" width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="20 6 9 17 4 12"/>
                  </svg>
                </div>
                <span class="checkbox-text">{{ t('auth.agreeTerms') }}</span>
              </label>
              <label class="checkbox-label" @click="setField('ageConfirm', !formData.ageConfirm)">
                <div class="checkbox-box" :class="{ checked: formData.ageConfirm }">
                  <svg v-if="formData.ageConfirm" width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="20 6 9 17 4 12"/>
                  </svg>
                </div>
                <span class="checkbox-text">{{ t('auth.ageConfirm') }}</span>
              </label>
            </div>
          </template>

          <!-- Login fields -->
          <template v-else>
            <div class="form-field">
              <label class="field-label">{{ t('auth.email') }}</label>
              <input
                type="email"
                class="field-input ltr"
                :class="{ error: hasError('email') }"
                :placeholder="t('auth.emailInputPlaceholder')"
                :value="formData.email"
                @input="(e) => setField('email', (e.target as HTMLInputElement).value)"
                @blur="() => blur('email')"
              />
              <span v-if="hasError('email')" class="field-error">{{ errors.email }}</span>
            </div>
            <div class="form-field">
              <label class="field-label">{{ t('auth.password') }}</label>
              <div class="password-wrapper">
                <input
                  :type="showPass ? 'text' : 'password'"
                  class="field-input ltr"
                  :class="{ error: hasError('password') }"
                  :placeholder="t('auth.passwordPlaceholder')"
                  :value="formData.password"
                  @input="(e) => setField('password', (e.target as HTMLInputElement).value)"
                  @blur="() => blur('password')"
                />
                <button type="button" class="eye-btn" @click="showPass = !showPass">
                  <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                    <template v-if="showPass">
                      <path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94"/>
                      <path d="M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19"/>
                      <line x1="1" y1="1" x2="23" y2="23"/>
                    </template>
                    <template v-else>
                      <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
                      <circle cx="12" cy="12" r="3"/>
                    </template>
                  </svg>
                </button>
              </div>
              <span v-if="hasError('password')" class="field-error">{{ errors.password }}</span>
            </div>

            <!-- Forgot password -->
            <div class="forgot-row">
              <button type="button" class="forgot-link" @click="goToForgotPassword">
                {{ t('auth.forgotPassword') }}
              </button>
            </div>
          </template>

          <!-- Error message -->
          <div v-if="authStore.error" class="auth-error">
            {{ authStore.error }}
          </div>

          <!-- Invisible ARCaptcha container (only for signup) -->
          <div v-if="isSignup" id="arcaptcha-container" style="display:none;"></div>

          <!-- Submit button -->
          <button
            type="submit"
            class="submit-btn"
            :disabled="!isFormReady || authStore.loading || captchaLoading"
          >
            <span v-if="authStore.loading || captchaLoading" class="spinner" />
            {{ isSignup ? t('auth.signup') : t('auth.loginButton') }}
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M5 12h14"/>
              <path d="M12 5l7 7-7 7"/>
            </svg>
          </button>

          <!-- Divider -->
          <div class="divider">
            <div class="divider-line" />
            <span class="divider-text">{{ t('auth.or') }}</span>
            <div class="divider-line" />
          </div>

          <!-- Social login -->
          <button
            type="button"
            class="social-btn"
            :disabled="googleLoading"
            @click="handleGoogleLogin"
          >
            <span v-if="googleLoading" class="btn-spinner" />
            <svg v-else width="18" height="18" viewBox="0 0 24 24">
              <path fill="#EA4335" d="M5.26620003,9.76452941 C6.19878754,6.93863203 8.85444915,4.90909091 12,4.90909091 C13.6909091,4.90909091 15.2181818,5.50909091 16.4181818,6.49090909 L19.9090909,3 C17.7818182,1.14545455 15.0545455,0 12,0 C7.27006974,0 3.1977497,2.69829785 1.23999023,6.65002441 L5.26620003,9.76452941 Z"/>
              <path fill="#34A853" d="M16.0407269,18.0125889 C14.9509167,18.7163016 13.5660892,19.0909091 12,19.0909091 C8.86648613,19.0909091 6.21911939,17.076871 5.27698177,14.2678769 L1.23746264,17.3349879 C3.19279051,21.2936293 7.26500293,24 12,24 C14.9328362,24 17.7353462,22.9573905 19.834192,20.9995801 L16.0407269,18.0125889 Z"/>
              <path fill="#4A90E2" d="M19.834192,20.9995801 C22.0291676,18.9520994 23.4545455,15.903663 23.4545455,12 C23.4545455,11.2909091 23.3454545,10.5272727 23.1818182,9.81818182 L12,9.81818182 L12,14.4545455 L18.4363636,14.4545455 C18.1187732,16.013626 17.2662994,17.2212117 16.0407269,18.0125889 L19.834192,20.9995801 Z"/>
              <path fill="#FBBC05" d="M5.27698177,14.2678769 C5.03832634,13.556323 4.90909091,12.7937589 4.90909091,12 C4.90909091,11.2182781 5.03443647,10.4668121 5.26620003,9.76452941 L1.23999023,6.65002441 C0.43658717,8.26043162 0,10.0753848 0,12 C0,13.9195484 0.444780743,15.7363265 1.2341904,17.3488889 L5.27698177,14.2678769 Z"/>
            </svg>
            {{ t('auth.google') }}
          </button>
        </form>

        <!-- Toggle mode -->
        <p class="toggle-text">
          {{ isSignup ? t('auth.hasAccount') : t('auth.noAccount') }}
          <button type="button" class="toggle-link" @click="toggleMode">
            {{ isSignup ? t('auth.switchLogin') : t('auth.switchSignup') }}
          </button>
        </p>
      </div>
    </div>

    <!-- Verification Modal Flow -->
    <VerificationFlow
      v-if="showVerification"
      :available-methods="verificationMethods"
      :masked-phone="verificationMaskedPhone"
      :masked-email="verificationMaskedEmail"
      :user-name="authStore.user?.display_name"
      :user-email="authStore.user?.email"
      @verified="onVerified"
      @close="onVerificationClose"
    />
  </div>
</template>

<style scoped>
.login-root {
  min-height: 100vh;
  min-height: 100dvh;
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--theme-bg);
  font-family: 'DM Sans', sans-serif;
  position: relative;
  padding: 16px;
}

.bg-image {
  position: fixed;
  inset: 0;
  background: linear-gradient(135deg, #0a1628 0%, #0d1f1a 30%, #121a12 60%, #0a0f1a 100%);
  pointer-events: none;
  z-index: 0;
}

.bg-overlay {
  position: fixed;
  inset: 0;
  background: rgba(6, 11, 8, 0.45);
  pointer-events: none;
  z-index: 1;
}

.grid-overlay {
  position: fixed;
  inset: 0;
  background-image: linear-gradient(rgba(255, 255, 255, 0.015) 1px, transparent 1px),
                    linear-gradient(90deg, rgba(255, 255, 255, 0.015) 1px, transparent 1px);
  background-size: 70px 70px;
  pointer-events: none;
  z-index: 2;
}

.login-card {
  width: 100%;
  max-width: 440px;
  position: relative;
  z-index: 10;
  opacity: 0;
  transform: translateY(20px);
  transition: all 0.7s cubic-bezier(0.16, 1, 0.3, 1), max-width 0.5s cubic-bezier(0.16, 1, 0.3, 1);
}

.login-card.mounted {
  opacity: 1;
  transform: none;
}

.login-card.signup {
  max-width: 620px;
}

.card-inner {
  background: rgba(12, 20, 16, 0.85);
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: 22px;
  padding: 42px 38px;
  backdrop-filter: blur(40px);
  position: relative;
  overflow: hidden;
}

.card-inner.signup {
  padding: 28px 32px;
}

.accent-line {
  position: absolute;
  top: 0;
  left: 32px;
  right: 32px;
  height: 1px;
  background: linear-gradient(90deg, transparent, #c89564, transparent);
  opacity: 0.4;
}

.lang-row {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  margin-bottom: 16px;
}

.card-inner.signup .lang-row {
  margin-bottom: 8px;
}

.lang-buttons {
  display: flex;
  gap: 4px;
}

.lang-btn {
  padding: 4px 11px;
  font-size: 12px;
  font-weight: 400;
  letter-spacing: 0.5px;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 6px;
  color: rgba(255, 255, 255, 0.3);
  cursor: pointer;
  font-family: 'DM Sans', sans-serif;
  transition: all 0.3s;
  line-height: 20px;
}

.lang-btn.active {
  font-weight: 600;
  background: rgba(200, 149, 100, 0.15);
  border-color: rgba(200, 149, 100, 0.3);
  color: #c89564;
}

.brand-header {
  text-align: center;
  margin-bottom: 28px;
}

.brand-header.signup {
  margin-bottom: 16px;
}

.logo-wrapper {
  width: 38px;
  height: 38px;
  margin: 0 auto 12px;
}

.logo-box {
  width: 100%;
  height: 100%;
  border: 1.5px solid rgba(200, 149, 100, 0.4);
  border-radius: 8px;
  transform: rotate(45deg);
  display: flex;
  align-items: center;
  justify-content: center;
  transition: border-color 0.4s;
}

.logo-dot {
  width: 4px;
  height: 4px;
  background: #c89564;
  border-radius: 50%;
  transform: rotate(-45deg);
  transition: background 0.4s;
}

.brand-title {
  font-family: 'Syne', sans-serif;
  font-size: 28px;
  font-weight: 700;
  color: #f5f5f5;
  letter-spacing: -0.5px;
  margin-bottom: 4px;
}

.brand-header.signup .brand-title {
  font-size: 24px;
}

.brand-sub {
  font-size: 14px;
  color: rgba(255, 255, 255, 0.55);
  font-weight: 300;
  line-height: 1.5;
}

.auth-form {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.auth-form.signup {
  gap: 10px;
}

.form-row {
  display: flex;
  gap: 10px;
}

.form-field {
  flex: 1 1 100%;
  min-width: 0;
}

.form-field.half {
  flex: 1 1 calc(50% - 5px);
}

.field-label {
  display: block;
  font-size: 11px;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.35);
  text-transform: uppercase;
  letter-spacing: 1.2px;
  margin-bottom: 5px;
  font-family: 'Syne', sans-serif;
}

.field-input,
.field-select {
  width: 100%;
  padding: 11px 13px;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 10px;
  color: #f5f5f5;
  font-size: 15px;
  font-family: 'DM Sans', sans-serif;
  outline: none;
  transition: all 0.3s;
  box-sizing: border-box;
}

.field-input::placeholder {
  color: rgba(255, 255, 255, 0.2);
}

.field-input:focus,
.field-select:focus {
  border-color: rgba(200, 149, 100, 0.3);
  background: rgba(255, 255, 255, 0.06);
  box-shadow: 0 0 0 2px rgba(200, 149, 100, 0.08);
}

.field-input.error,
.field-select.error {
  border-color: rgba(239, 68, 68, 0.5);
}

.field-input.ltr {
  direction: ltr;
  text-align: left;
}

.field-select {
  appearance: none;
  cursor: pointer;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='rgba(255,255,255,0.3)' stroke-width='2'%3E%3Cpolyline points='6 9 12 15 18 9'%3E%3C/polyline%3E%3C/svg%3E");
  background-repeat: no-repeat;
  background-position: right 12px center;
  padding-right: 36px;
}

[dir="rtl"] .field-select {
  background-position: left 12px center;
  padding-right: 13px;
  padding-left: 36px;
}

.field-select option {
  background: #0e1612;
  color: #f5f5f5;
}

.field-error {
  font-size: 11px;
  color: #ef4444;
  margin-top: 2px;
  display: block;
  font-weight: 400;
}

.password-wrapper {
  position: relative;
}

.password-wrapper .field-input {
  padding-right: 40px;
}

[dir="rtl"] .password-wrapper .field-input {
  padding-right: 13px;
  padding-left: 40px;
}

.eye-btn {
  position: absolute;
  right: 8px;
  top: 50%;
  transform: translateY(-50%);
  background: none;
  border: none;
  color: rgba(255, 255, 255, 0.25);
  cursor: pointer;
  padding: 2px;
  display: flex;
}

[dir="rtl"] .eye-btn {
  right: auto;
  left: 8px;
}

/* Phone input styles */
.phone-input-row {
  display: flex;
  position: relative;
  direction: ltr;
}

.phone-code-btn {
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-right: none;
  border-radius: 10px 0 0 10px;
  padding: 0 8px;
  display: flex;
  align-items: center;
  gap: 5px;
  cursor: pointer;
  white-space: nowrap;
  color: #f5f5f5;
  font-family: 'DM Sans', sans-serif;
  transition: all 0.3s;
  min-width: 80px;
  box-sizing: border-box;
  flex-shrink: 0;
}

.phone-code-btn.error {
  border-color: rgba(239, 68, 68, 0.5);
}

.phone-flag {
  font-size: 16px;
  line-height: 1;
}

.phone-dial {
  color: rgba(255, 255, 255, 0.55);
  font-size: 12px;
}

.phone-arrow {
  font-size: 7px;
  color: rgba(255, 255, 255, 0.2);
  margin-left: 2px;
}

.phone-number {
  border-radius: 0 10px 10px 0;
  flex: 1;
}

.phone-dropdown {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  z-index: 200;
  width: 280px;
  max-height: 240px;
  display: flex;
  flex-direction: column;
  background: rgba(14, 22, 18, 0.98);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  box-shadow: 0 16px 48px rgba(0, 0, 0, 0.6);
  overflow: hidden;
}

.phone-search-wrapper {
  padding: 8px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  flex-shrink: 0;
}

.phone-search {
  width: 100%;
  padding: 8px 10px;
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 7px;
  color: #f5f5f5;
  font-size: 13px;
  outline: none;
  font-family: 'DM Sans', sans-serif;
  direction: ltr;
  text-align: left;
  box-sizing: border-box;
}

.phone-list {
  overflow-y: auto;
  flex: 1;
}

.phone-no-results {
  padding: 14px;
  text-align: center;
  color: rgba(255, 255, 255, 0.25);
  font-size: 13px;
}

.phone-option {
  padding: 8px 12px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.7);
  transition: background 0.15s;
}

.phone-option:hover {
  background: rgba(255, 255, 255, 0.06);
}

.phone-option.selected {
  background: rgba(200, 149, 100, 0.1);
}

.phone-option-flag {
  font-size: 16px;
}

.phone-option-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.phone-option-dial {
  color: rgba(255, 255, 255, 0.3);
  font-size: 12px;
  flex-shrink: 0;
}

/* Checkboxes */
.checkbox-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 2px;
}

.checkbox-label {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  cursor: pointer;
}

.checkbox-box {
  width: 17px;
  height: 17px;
  min-width: 17px;
  border-radius: 5px;
  border: 1.5px solid rgba(255, 255, 255, 0.15);
  background: transparent;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
  margin-top: 1px;
  color: #c89564;
}

.checkbox-box.checked {
  border-color: #c89564;
  background: rgba(200, 149, 100, 0.15);
}

.checkbox-text {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.45);
  font-weight: 300;
  line-height: 1.4;
  user-select: none;
}

/* Forgot password */
.forgot-row {
  display: flex;
  justify-content: flex-end;
}

[dir="rtl"] .forgot-row {
  justify-content: flex-start;
}

.forgot-link {
  background: none;
  border: none;
  color: #c89564;
  font-size: 13px;
  font-weight: 400;
  cursor: pointer;
  font-family: 'DM Sans', sans-serif;
  padding: 0;
  transition: color 0.3s;
}

.forgot-link:hover {
  text-decoration: underline;
}

/* Auth error */
.auth-error {
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.3);
  border-radius: 8px;
  padding: 10px 14px;
  color: #ef4444;
  font-size: 13px;
  text-align: center;
}

/* Submit button */
.submit-btn {
  width: 100%;
  padding: 13px;
  background: linear-gradient(135deg, #c89564, #a37545);
  border: none;
  border-radius: 10px;
  color: #0a0f0c;
  font-size: 15px;
  font-weight: 600;
  font-family: 'Syne', sans-serif;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  transition: all 0.3s cubic-bezier(0.16, 1, 0.3, 1);
  letter-spacing: 0.3px;
}

.submit-btn:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 8px 24px rgba(200, 149, 100, 0.3);
}

.submit-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.spinner {
  width: 16px;
  height: 16px;
  border: 2px solid transparent;
  border-top-color: currentColor;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

.btn-spinner {
  width: 18px;
  height: 18px;
  border: 2px solid rgba(255, 255, 255, 0.2);
  border-top-color: rgba(255, 255, 255, 0.6);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

/* Divider */
.divider {
  display: flex;
  align-items: center;
  gap: 14px;
  margin: 6px 0;
}

.divider-line {
  flex: 1;
  height: 1px;
  background: rgba(255, 255, 255, 0.06);
}

.divider-text {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.2);
  text-transform: uppercase;
  letter-spacing: 2px;
  font-family: 'Syne', sans-serif;
}

/* Social button */
.social-btn {
  flex: 1;
  padding: 11px;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 10px;
  color: rgba(255, 255, 255, 0.5);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.3s;
  font-family: 'DM Sans', sans-serif;
  font-size: 14px;
  gap: 7px;
}

.social-btn:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.08);
  border-color: rgba(255, 255, 255, 0.15);
}

.social-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

/* Toggle mode */
.toggle-text {
  text-align: center;
  margin-top: 16px;
  font-size: 14px;
  color: rgba(255, 255, 255, 0.55);
  font-weight: 300;
}

.toggle-link {
  background: none;
  border: none;
  color: #c89564;
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  font-family: 'DM Sans', sans-serif;
  padding: 0;
  margin-inline-start: 5px;
  transition: color 0.3s;
}

.toggle-link:hover {
  text-decoration: underline;
}

/* Scrollbar */
::-webkit-scrollbar {
  width: 4px;
}

::-webkit-scrollbar-track {
  background: transparent;
}

::-webkit-scrollbar-thumb {
  background: rgba(200, 149, 100, 0.3);
  border-radius: 4px;
}

/* Responsive */
@media (max-width: 560px) {
  .form-row {
    flex-direction: column;
    gap: 8px;
  }

  .form-field.half {
    flex: 1 1 100%;
    min-width: 100%;
  }

  .card-inner {
    padding: 28px 20px;
  }

  .card-inner.signup {
    padding: 24px 18px;
  }
}
</style>
