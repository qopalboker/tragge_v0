import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import type { Locale, Direction } from '../i18n';
import { setLocale as setI18nLocale, getLocale } from '../i18n';

export const useI18nStore = defineStore('i18n', () => {
  const locale = ref<Locale>(getLocale());

  const direction = computed<Direction>(() =>
    locale.value === 'fa' ? 'rtl' : 'ltr'
  );

  const isRtl = computed(() => direction.value === 'rtl');

  function setLocale(newLocale: Locale): void {
    locale.value = newLocale;
    setI18nLocale(newLocale);
  }

  function toggleLocale(): void {
    setLocale(locale.value === 'en' ? 'fa' : 'en');
  }

  return {
    locale,
    direction,
    isRtl,
    setLocale,
    toggleLocale,
  };
});
