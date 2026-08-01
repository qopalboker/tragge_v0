import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import { api } from '@/api';

const REFERRAL_CODE_KEY = 'referral_code';

export interface ReferralValidationResult {
  valid: boolean;
  referrer_name?: string;
  error?: string;
}

export const useReferralStore = defineStore('referral', () => {
  const referralCode = ref<string | null>(localStorage.getItem(REFERRAL_CODE_KEY));
  const referrerName = ref<string | null>(null);
  const isValidating = ref(false);
  const isValid = ref<boolean | null>(null);
  const validationError = ref<string | null>(null);
  const fromUrl = ref(false);

  const hasReferralCode = computed(() => !!referralCode.value);

  /**
   * Capture referral code from URL query parameter
   */
  function captureFromUrl(): void {
    const urlParams = new URLSearchParams(window.location.search);
    const refCode = urlParams.get('ref');

    if (refCode) {
      setReferralCode(refCode, true);
    }
  }

  /**
   * Set the referral code and persist to localStorage
   */
  function setReferralCode(code: string | null, isFromUrl = false): void {
    referralCode.value = code;
    fromUrl.value = isFromUrl;

    if (code) {
      localStorage.setItem(REFERRAL_CODE_KEY, code);
    } else {
      localStorage.removeItem(REFERRAL_CODE_KEY);
    }

    // Reset validation state when code changes
    isValid.value = null;
    validationError.value = null;
    referrerName.value = null;
  }

  /**
   * Validate the referral code with the backend
   */
  async function validateCode(code?: string): Promise<ReferralValidationResult> {
    const codeToValidate = code || referralCode.value;

    if (!codeToValidate) {
      return { valid: false, error: 'No referral code provided' };
    }

    isValidating.value = true;
    validationError.value = null;

    try {
      const response = await api.get<{ valid: boolean; referrer_name?: string; error?: string }>(
        '/api/user/referral/validate',
        { params: { code: codeToValidate } }
      );

      isValid.value = response.data.valid;

      if (response.data.valid) {
        referrerName.value = response.data.referrer_name || null;
        validationError.value = null;
      } else {
        referrerName.value = null;
        validationError.value = response.data.error || 'Invalid referral code';
      }

      return {
        valid: response.data.valid,
        referrer_name: response.data.referrer_name,
        error: response.data.error,
      };
    } catch (err: unknown) {
      isValid.value = false;

      if (err && typeof err === 'object' && 'response' in err) {
        const axiosError = err as { response?: { data?: { error?: string }; status?: number } };

        if (axiosError.response?.status === 404) {
          validationError.value = 'Referral code not found';
        } else if (axiosError.response?.status === 410) {
          validationError.value = 'Referral code has expired';
        } else {
          validationError.value = axiosError.response?.data?.error || 'Invalid referral code';
        }
      } else {
        validationError.value = 'Failed to validate referral code';
      }

      return {
        valid: false,
        error: validationError.value,
      };
    } finally {
      isValidating.value = false;
    }
  }

  /**
   * Clear the referral code and all related state
   */
  function clearReferralCode(): void {
    referralCode.value = null;
    referrerName.value = null;
    isValid.value = null;
    validationError.value = null;
    fromUrl.value = false;
    localStorage.removeItem(REFERRAL_CODE_KEY);
  }

  return {
    referralCode,
    referrerName,
    isValidating,
    isValid,
    validationError,
    fromUrl,
    hasReferralCode,
    captureFromUrl,
    setReferralCode,
    validateCode,
    clearReferralCode,
  };
});
