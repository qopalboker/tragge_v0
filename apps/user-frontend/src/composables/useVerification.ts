import { ref, onUnmounted } from 'vue';
import { api } from '@/api';

export type VerificationMethod = 'sms' | 'email';

interface SendVerificationResponse {
  message: string;
  destination_masked: string;
  expires_in_seconds: number;
  resend_cooldown_seconds: number;
}

interface VerifyCodeResponse {
  message: string;
}

interface ApiErrorResponse {
  response?: {
    status?: number;
    data?: {
      error?: string;
      message?: string;
      remaining_attempts?: number;
      retry_after_seconds?: number;
    };
  };
}

export function useVerification() {
  const loading = ref(false);
  const error = ref<string | null>(null);
  const remainingAttempts = ref(5);
  const resendCooldown = ref(0);
  const maskedDestination = ref('');

  let cooldownInterval: ReturnType<typeof setInterval> | null = null;

  function startCooldown(seconds: number) {
    resendCooldown.value = seconds;
    if (cooldownInterval) clearInterval(cooldownInterval);
    cooldownInterval = setInterval(() => {
      if (resendCooldown.value > 0) {
        resendCooldown.value--;
      } else {
        if (cooldownInterval) clearInterval(cooldownInterval);
      }
    }, 1000);
  }

  function stopCooldown() {
    if (cooldownInterval) {
      clearInterval(cooldownInterval);
      cooldownInterval = null;
    }
  }

  async function sendCode(method: VerificationMethod): Promise<boolean> {
    loading.value = true;
    error.value = null;

    try {
      const response = await api.post<SendVerificationResponse>(
        '/api/user/auth/send-verification',
        { method },
      );
      maskedDestination.value = response.data.destination_masked;
      startCooldown(response.data.resend_cooldown_seconds);
      remainingAttempts.value = 5;
      return true;
    } catch (err: unknown) {
      const apiErr = err as ApiErrorResponse;
      const errCode = apiErr.response?.data?.error;

      if (errCode === 'rate_limit_exceeded') {
        const retry = apiErr.response?.data?.retry_after_seconds ?? 60;
        startCooldown(retry);
        error.value = null; // Not an error, just rate limited
      } else if (errCode === 'already_verified') {
        error.value = null;
        return true; // Already verified, treat as success
      } else {
        error.value = apiErr.response?.data?.message || 'Failed to send code';
      }
      return false;
    } finally {
      loading.value = false;
    }
  }

  async function verifyCode(code: string): Promise<boolean> {
    loading.value = true;
    error.value = null;

    try {
      await api.post<VerifyCodeResponse>('/api/user/auth/verify-code', { code });
      return true;
    } catch (err: unknown) {
      const apiErr = err as ApiErrorResponse;
      const errCode = apiErr.response?.data?.error;

      if (errCode === 'invalid_code' || errCode === 'wrong_code') {
        error.value = apiErr.response?.data?.message || 'Invalid code';
        remainingAttempts.value = apiErr.response?.data?.remaining_attempts ?? remainingAttempts.value - 1;
      } else if (errCode === 'code_exhausted' || errCode === 'no_valid_code') {
        error.value = apiErr.response?.data?.message || 'Code expired';
        remainingAttempts.value = 0;
      } else if (errCode === 'too_many_attempts') {
        error.value = apiErr.response?.data?.message || 'Too many attempts';
        remainingAttempts.value = 0;
      } else {
        error.value = apiErr.response?.data?.message || 'Verification failed';
      }
      return false;
    } finally {
      loading.value = false;
    }
  }

  async function resendCode(method: VerificationMethod): Promise<boolean> {
    return sendCode(method);
  }

  function cleanup() {
    stopCooldown();
  }

  onUnmounted(() => {
    stopCooldown();
  });

  return {
    loading,
    error,
    remainingAttempts,
    resendCooldown,
    maskedDestination,
    sendCode,
    verifyCode,
    resendCode,
    cleanup,
  };
}
