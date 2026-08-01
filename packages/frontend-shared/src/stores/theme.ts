import { defineStore } from 'pinia';
import { ref, computed, watch } from 'vue';

export type Theme = 'light' | 'dark' | 'system';
export type ResolvedTheme = 'light' | 'dark';
export type ThemeId = 'dark' | 'darkOcean' | 'darkEmber' | 'light' | 'lightMint' | 'lightRose';

export interface ThemeColors {
  name: string;
  bg: string;
  surface: string;
  surfaceHover: string;
  glass: string;
  glassBorder: string;
  glassHighlight: string;
  text: string;
  textSecondary: string;
  accent: string;
  accentGlow: string;
  green: string;
  red: string;
  orange: string;
  gold: string;
  navBg: string;
  sidebarBg: string;
  isDark: boolean;
}

export const themes: Record<ThemeId, ThemeColors> = {
  dark: { name: "Obsidian", bg: "linear-gradient(145deg,#0a0a0f 0%,#111128 40%,#0d1117 100%)", surface: "rgba(255,255,255,0.04)", surfaceHover: "rgba(255,255,255,0.08)", glass: "rgba(255,255,255,0.06)", glassBorder: "rgba(255,255,255,0.1)", glassHighlight: "rgba(255,255,255,0.15)", text: "#e8e8f0", textSecondary: "#8888a8", accent: "#6c5ce7", accentGlow: "rgba(108,92,231,0.3)", green: "#00d2a0", red: "#ff4757", orange: "#ffa502", gold: "#ffd700", navBg: "rgba(10,10,20,0.85)", sidebarBg: "rgba(12,12,30,0.95)", isDark: true },
  darkOcean: { name: "Ocean", bg: "linear-gradient(145deg,#020c1b 0%,#0a192f 40%,#061220 100%)", surface: "rgba(100,200,255,0.04)", surfaceHover: "rgba(100,200,255,0.08)", glass: "rgba(100,200,255,0.05)", glassBorder: "rgba(100,200,255,0.12)", glassHighlight: "rgba(100,200,255,0.18)", text: "#ccd6f6", textSecondary: "#6886a8", accent: "#64ffda", accentGlow: "rgba(100,255,218,0.2)", green: "#64ffda", red: "#ff6b81", orange: "#f7b731", gold: "#ffd700", navBg: "rgba(2,12,27,0.9)", sidebarBg: "rgba(6,18,32,0.95)", isDark: true },
  darkEmber: { name: "Ember", bg: "linear-gradient(145deg,#1a0a0a 0%,#2d1515 40%,#1a0e0e 100%)", surface: "rgba(255,120,80,0.04)", surfaceHover: "rgba(255,120,80,0.08)", glass: "rgba(255,120,80,0.05)", glassBorder: "rgba(255,120,80,0.1)", glassHighlight: "rgba(255,120,80,0.15)", text: "#f0e0d8", textSecondary: "#a88878", accent: "#ff6b35", accentGlow: "rgba(255,107,53,0.25)", green: "#2ed573", red: "#ff4757", orange: "#ff9f43", gold: "#ffd700", navBg: "rgba(26,10,10,0.9)", sidebarBg: "rgba(30,14,14,0.95)", isDark: true },
  light: { name: "Cloud", bg: "linear-gradient(145deg,#f0f2f8 0%,#e8ecf4 40%,#f5f6fa 100%)", surface: "rgba(0,0,0,0.03)", surfaceHover: "rgba(0,0,0,0.06)", glass: "rgba(255,255,255,0.6)", glassBorder: "rgba(0,0,0,0.08)", glassHighlight: "rgba(255,255,255,0.9)", text: "#1a1a2e", textSecondary: "#6b7280", accent: "#6c5ce7", accentGlow: "rgba(108,92,231,0.15)", green: "#00b894", red: "#e74c3c", orange: "#e67e22", gold: "#f39c12", navBg: "rgba(255,255,255,0.85)", sidebarBg: "rgba(248,249,252,0.97)", isDark: false },
  lightMint: { name: "Mint", bg: "linear-gradient(145deg,#e8f5f0 0%,#d4ede4 40%,#eef8f4 100%)", surface: "rgba(0,80,60,0.03)", surfaceHover: "rgba(0,80,60,0.06)", glass: "rgba(255,255,255,0.55)", glassBorder: "rgba(0,80,60,0.08)", glassHighlight: "rgba(255,255,255,0.85)", text: "#1a2e28", textSecondary: "#5f8078", accent: "#00b894", accentGlow: "rgba(0,184,148,0.15)", green: "#00b894", red: "#e74c3c", orange: "#e67e22", gold: "#f39c12", navBg: "rgba(255,255,255,0.88)", sidebarBg: "rgba(245,252,248,0.97)", isDark: false },
  lightRose: { name: "Rosé", bg: "linear-gradient(145deg,#fdf2f8 0%,#f8e4ef 40%,#fef5fa 100%)", surface: "rgba(180,0,80,0.03)", surfaceHover: "rgba(180,0,80,0.06)", glass: "rgba(255,255,255,0.55)", glassBorder: "rgba(180,0,80,0.08)", glassHighlight: "rgba(255,255,255,0.85)", text: "#2e1a24", textSecondary: "#9c7088", accent: "#e84393", accentGlow: "rgba(232,67,147,0.15)", green: "#00b894", red: "#e74c3c", orange: "#e67e22", gold: "#f39c12", navBg: "rgba(255,255,255,0.88)", sidebarBg: "rgba(253,245,250,0.97)", isDark: false },
};

const STORAGE_KEY = 'tragge-theme';

function migrateOldTheme(stored: string): ThemeId {
  if (stored === 'system' || stored === 'dark') return 'dark';
  if (stored === 'light') return 'light';
  if (stored in themes) return stored as ThemeId;
  return 'dark';
}

export const useThemeStore = defineStore('theme', () => {
  const themeId = ref<ThemeId>(getStoredTheme());

  const currentTheme = computed<ThemeColors>(() => themes[themeId.value]);
  const isDark = computed(() => currentTheme.value.isDark);
  const resolvedTheme = computed<ResolvedTheme>(() => isDark.value ? 'dark' : 'light');
  const theme = ref<Theme>(isDark.value ? 'dark' : 'light');

  function getStoredTheme(): ThemeId {
    if (typeof window === 'undefined') return 'dark';
    const stored = localStorage.getItem(STORAGE_KEY);
    if (!stored) return 'dark';
    return migrateOldTheme(stored);
  }

  function setTheme(newTheme: ThemeId | Theme): void {
    if (newTheme === 'system') {
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
      themeId.value = prefersDark ? 'dark' : 'light';
    } else if (newTheme in themes) {
      themeId.value = newTheme as ThemeId;
    } else {
      themeId.value = newTheme === 'light' ? 'light' : 'dark';
    }
    localStorage.setItem(STORAGE_KEY, themeId.value);
    theme.value = isDark.value ? 'dark' : 'light';
    applyTheme();
  }

  function toggleTheme(): void {
    const allThemes: ThemeId[] = ['dark', 'darkOcean', 'darkEmber', 'light', 'lightMint', 'lightRose'];
    const currentIndex = allThemes.indexOf(themeId.value);
    const nextIndex = (currentIndex + 1) % allThemes.length;
    setTheme(allThemes[nextIndex]);
  }

  function applyTheme(): void {
    const root = document.documentElement;
    const t = currentTheme.value;

    if (t.isDark) {
      root.classList.add('dark');
    } else {
      root.classList.remove('dark');
    }

    root.style.setProperty('--theme-bg', t.bg);
    root.style.setProperty('--theme-surface', t.surface);
    root.style.setProperty('--theme-surface-hover', t.surfaceHover);
    root.style.setProperty('--theme-glass', t.glass);
    root.style.setProperty('--theme-glass-border', t.glassBorder);
    root.style.setProperty('--theme-glass-highlight', t.glassHighlight);
    root.style.setProperty('--theme-text', t.text);
    root.style.setProperty('--theme-text-secondary', t.textSecondary);
    root.style.setProperty('--theme-accent', t.accent);
    root.style.setProperty('--theme-accent-glow', t.accentGlow);
    root.style.setProperty('--theme-green', t.green);
    root.style.setProperty('--theme-red', t.red);
    root.style.setProperty('--theme-orange', t.orange);
    root.style.setProperty('--theme-gold', t.gold);
    root.style.setProperty('--theme-nav-bg', t.navBg);
    root.style.setProperty('--theme-sidebar-bg', t.sidebarBg);
  }

  function initTheme(): void {
    window.addEventListener('storage', (event) => {
      if (event.key === STORAGE_KEY && event.newValue) {
        themeId.value = migrateOldTheme(event.newValue);
        applyTheme();
      }
    });

    applyTheme();
  }

  watch(themeId, () => {
    theme.value = isDark.value ? 'dark' : 'light';
    applyTheme();
  });

  return {
    theme,
    themeId,
    currentTheme,
    resolvedTheme,
    isDark,
    setTheme,
    toggleTheme,
    initTheme,
  };
});
