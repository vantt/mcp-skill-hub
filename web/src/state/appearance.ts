import { useEffect, useState } from 'react';
import { globalKey, readJSON, writeJSON } from './local-store';

export type Scheme = 'light' | 'dark' | 'system';

export interface ThemeInfo {
  id: string;
  name: string;
  note: string;
}

export const THEMES: ThemeInfo[] = [
  { id: 'precision', name: 'Precision', note: 'Sharp, border-first' },
  { id: 'berich', name: 'beRich', note: 'Warm, gemstone accents' },
  { id: 'clickup', name: 'ClickUp', note: 'Rounded, violet brand' },
  { id: 'terminal', name: 'Terminal', note: 'Mono, square corners' },
  { id: 'atelier', name: 'Atelier', note: 'Soft, pill radius' },
  { id: 'moday', name: 'Moday', note: 'Friendly, open' },
];

export const ACCENTS: Record<string, string[]> = {
  precision: ['moss', 'honey'],
  berich: ['gold', 'sapphire', 'topaz', 'amethyst', 'garnet'],
  clickup: ['violet', 'blue'],
  terminal: ['amber'],
  atelier: ['clay', 'honey'],
  moday: ['purple', 'green'],
};

export interface AppearanceState {
  scheme: Scheme;
  theme: string;
  accent: string;
}

const FONT_LOADERS: Record<string, () => Promise<unknown>> = {
  precision: () => import('../design-system/fonts/precision'),
  berich: () => import('../design-system/fonts/berich'),
  clickup: () => import('../design-system/fonts/clickup'),
  terminal: () => import('../design-system/fonts/terminal'),
  atelier: () => import('../design-system/fonts/atelier'),
  moday: () => import('../design-system/fonts/moday'),
};

const APPEARANCE_KEY = globalKey('appearance');

export const DEFAULT_APPEARANCE: AppearanceState = {
  scheme: 'system',
  theme: 'precision',
  accent: ACCENTS.precision?.[0] ?? 'moss',
};

const appearanceEvents = new EventTarget();

export function getStoredAppearance(): AppearanceState {
  const stored = readJSON<AppearanceState>(APPEARANCE_KEY);
  if (!stored) {
    return DEFAULT_APPEARANCE;
  }
  const theme = THEMES.some((t) => t.id === stored.theme) ? stored.theme : DEFAULT_APPEARANCE.theme;
  const validAccents = ACCENTS[theme] ?? [];
  const accent = validAccents.includes(stored.accent)
    ? stored.accent
    : (validAccents[0] ?? DEFAULT_APPEARANCE.accent);
  const scheme = ['light', 'dark', 'system'].includes(stored.scheme)
    ? stored.scheme
    : DEFAULT_APPEARANCE.scheme;
  return { scheme, theme, accent };
}

export function resolveSystemScheme(): 'light' | 'dark' {
  if (typeof window === 'undefined' || !window.matchMedia) {
    return 'light';
  }
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export function applyAppearance(state: AppearanceState): void {
  if (typeof document === 'undefined') {
    return;
  }

  const root = document.documentElement;
  root.setAttribute('data-theme', state.theme);
  root.setAttribute('data-accent', state.accent);

  const resolvedScheme = state.scheme === 'system' ? resolveSystemScheme() : state.scheme;
  root.setAttribute('data-scheme', resolvedScheme);

  const loader = FONT_LOADERS[state.theme];
  if (loader) {
    loader().catch(() => {
      // Font load error fallback to system fonts.
    });
  }
}

export function saveAppearance(next: AppearanceState): void {
  writeJSON(APPEARANCE_KEY, next);
  applyAppearance(next);
  appearanceEvents.dispatchEvent(new Event('appearance-changed'));
}

export function initAppearance(): void {
  const current = getStoredAppearance();
  applyAppearance(current);

  if (typeof window !== 'undefined' && window.matchMedia) {
    const mql = window.matchMedia('(prefers-color-scheme: dark)');
    mql.addEventListener('change', () => {
      const active = getStoredAppearance();
      if (active.scheme === 'system') {
        applyAppearance(active);
      }
    });
  }
}

export function useAppearance(): [AppearanceState, (next: AppearanceState) => void] {
  const [appearance, setAppearanceState] = useState<AppearanceState>(getStoredAppearance);

  useEffect(() => {
    const handler = () => {
      setAppearanceState(getStoredAppearance());
    };
    appearanceEvents.addEventListener('appearance-changed', handler);
    return () => {
      appearanceEvents.removeEventListener('appearance-changed', handler);
    };
  }, []);

  const update = (next: AppearanceState) => {
    saveAppearance(next);
    setAppearanceState(next);
  };

  return [appearance, update];
}
