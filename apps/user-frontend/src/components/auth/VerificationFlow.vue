<script setup lang="ts">
import { ref, onUnmounted } from 'vue';
import { useVerification, type VerificationMethod } from '@/composables/useVerification';
import VerificationMethodModal from './VerificationMethodModal.vue';
import VerificationCodeModal from './VerificationCodeModal.vue';
import VerificationSuccessModal from './VerificationSuccessModal.vue';

const props = defineProps<{
  availableMethods: string[];
  maskedPhone?: string;
  maskedEmail?: string;
  userName?: string;
  userEmail?: string;
}>();

const emit = defineEmits<{
  verified: [];
  close: [];
}>();

type Step = 'method' | 'code' | 'success';
const step = ref<Step>('method');
const selectedMethod = ref<VerificationMethod>('email');

const {
  loading,
  error,
  remainingAttempts,
  resendCooldown,
  maskedDestination,
  sendCode,
  verifyCode,
  resendCode,
  cleanup,
} = useVerification();

const codeModalRef = ref<InstanceType<typeof VerificationCodeModal> | null>(null);

async function onMethodSelect(method: VerificationMethod) {
  selectedMethod.value = method;
  const success = await sendCode(method);
  if (success) {
    step.value = 'code';
  }
}

async function onVerify(code: string) {
  const success = await verifyCode(code);
  if (success) {
    step.value = 'success';
  } else {
    // Clear inputs on failure
    codeModalRef.value?.clearAndFocus();
  }
}

async function onResend() {
  await resendCode(selectedMethod.value);
}

function onBack() {
  step.value = 'method';
  error.value = null;
}

function onContinue() {
  emit('verified');
}

function onClose() {
  emit('close');
}

// If only one method available, skip method selection
if (props.availableMethods.length === 1) {
  // Auto-select the only method, but still show method modal briefly for UX
}

onUnmounted(() => {
  cleanup();
});
</script>

<template>
  <VerificationMethodModal
    v-if="step === 'method'"
    :available-methods="availableMethods"
    :masked-phone="maskedPhone"
    :masked-email="maskedEmail"
    :loading="loading"
    @select="onMethodSelect"
    @close="onClose"
  />

  <VerificationCodeModal
    v-else-if="step === 'code'"
    ref="codeModalRef"
    :method="selectedMethod"
    :masked-destination="maskedDestination || (selectedMethod === 'email' ? maskedEmail : maskedPhone) || ''"
    :loading="loading"
    :error="error"
    :remaining-attempts="remainingAttempts"
    :resend-cooldown="resendCooldown"
    @verify="onVerify"
    @resend="onResend"
    @back="onBack"
  />

  <VerificationSuccessModal
    v-else-if="step === 'success'"
    :user-name="userName"
    :user-email="userEmail"
    @continue="onContinue"
  />
</template>
