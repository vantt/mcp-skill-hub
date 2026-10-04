import { useEffect, useRef, useState } from 'react';
import type { Scheme } from '../state/appearance';
import { ACCENTS, THEMES, useAppearance } from '../state/appearance';
import { useT } from '../i18n';

const THEME_PREVIEWS: Record<string, { bg: string; text: string; action: string; font: string }> = {
  precision: { bg: '#15151a', text: '#f0ece2', action: '#e8a341', font: 'var(--font-serif)' },
  berich: { bg: '#F7F3EA', text: '#211C16', action: '#0E6F52', font: 'var(--font-serif)' },
  clickup: { bg: '#ffffff', text: '#202020', action: '#202020', font: 'var(--font-display)' },
  terminal: { bg: '#000000', text: '#33ff66', action: '#33ff66', font: 'var(--font-mono)' },
  atelier: { bg: '#fdfaf5', text: '#26221f', action: '#e9590c', font: 'var(--font-sans)' },
  moday: { bg: '#ffffff', text: '#333333', action: '#0073ea', font: 'var(--font-sans)' },
};

const ACCENT_COLORS: Record<string, string> = {
  moss: '#5a7a50',
  honey: '#d49b38',
  gold: '#c8a04d',
  sapphire: '#28589c',
  topaz: '#2b7880',
  amethyst: '#704888',
  garnet: '#8e303c',
  violet: '#6647f0',
  blue: '#0091ff',
  amber: '#ffb000',
  clay: '#b85840',
  purple: '#8442c7',
  green: '#00854d',
};

const ICON_APP = '◩';
const SAMPLE_AA = 'Aa';
const CHECKMARK = '✓';

export function AppearanceMenu() {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [appearance, setAppearance] = useAppearance();
  const menuRef = useRef<HTMLDivElement>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setOpen(false);
        buttonRef.current?.focus();
      }
    };
    const handleClickOutside = (e: MouseEvent) => {
      if (
        menuRef.current &&
        !menuRef.current.contains(e.target as Node) &&
        !buttonRef.current?.contains(e.target as Node)
      ) {
        setOpen(false);
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    document.addEventListener('mousedown', handleClickOutside);
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, [open]);

  const activeTheme = appearance.theme;
  const activeAccents = ACCENTS[activeTheme] ?? [];

  const handleSchemePick = (scheme: Scheme) => {
    setAppearance({ ...appearance, scheme });
  };

  const handleThemePick = (themeId: string) => {
    const accents = ACCENTS[themeId] ?? [];
    const nextAccent = accents.includes(appearance.accent)
      ? appearance.accent
      : (accents[0] ?? 'default');
    setAppearance({ ...appearance, theme: themeId, accent: nextAccent });
  };

  const handleAccentPick = (accent: string) => {
    setAppearance({ ...appearance, accent });
  };

  const schemes: Array<{ id: Scheme; label: string; icon: string }> = [
    { id: 'light', label: t('appearance.scheme_light'), icon: '☼' },
    { id: 'dark', label: t('appearance.scheme_dark'), icon: '☽' },
    { id: 'system', label: t('appearance.scheme_system'), icon: '⚙' },
  ];

  const currentThemeInfo = THEMES.find((item) => item.id === activeTheme);
  const currentThemeName = currentThemeInfo ? currentThemeInfo.name : activeTheme;

  return (
    <div style={{ position: 'relative' }}>
      <button
        ref={buttonRef}
        type="button"
        className="fg-icon-btn"
        onClick={() => setOpen(!open)}
        title={t('appearance.menu_title')}
        aria-label={t('appearance.menu_title')}
        aria-haspopup="dialog"
        aria-expanded={open}
        style={{
          width: 'auto',
          height: '34px',
          padding: '0 10px',
          display: 'inline-flex',
          alignItems: 'center',
          gap: '8px',
        }}
      >
        <span aria-hidden="true" style={{ fontSize: '15px' }}>
          {ICON_APP}
        </span>
        <span
          style={{
            display: 'block',
            width: '12px',
            height: '12px',
            borderRadius: '999px',
            background: 'var(--color-action)',
          }}
        />
        <span aria-hidden="true" style={{ fontSize: '10px', color: 'var(--color-text-subtle)' }}>
          ▾
        </span>
      </button>

      {open && (
        <div
          ref={menuRef}
          className="fg-menu"
          role="dialog"
          aria-label={t('appearance.menu_title')}
          style={{
            position: 'absolute',
            top: 'calc(100% + 8px)',
            right: 0,
            width: 'min(560px, calc(100vw - 24px))',
            maxHeight: 'calc(100vh - 72px)',
            overflowY: 'auto',
            boxSizing: 'border-box',
            zIndex: 60,
            padding: 'var(--space-4)',
            display: 'flex',
            flexDirection: 'column',
            gap: 'var(--space-4)',
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span className="t-heading-sm">
              <span>{t('appearance.menu_title')}</span>
            </span>
            <button
              type="button"
              className="fg-icon-btn"
              aria-label={t('action.close')}
              onClick={() => setOpen(false)}
            >
              ✕
            </button>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
            <span className="t-label" style={{ color: 'var(--color-text-subtle)' }}>
              <span>{t('appearance.scheme')}</span>
            </span>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 'var(--space-2)' }}>
              {schemes.map((s) => {
                const isSelected = appearance.scheme === s.id;
                return (
                  <button
                    key={s.id}
                    type="button"
                    onClick={() => handleSchemePick(s.id)}
                    role="radio"
                    aria-checked={isSelected}
                    style={{
                      boxSizing: 'border-box',
                      cursor: 'pointer',
                      display: 'flex',
                      alignItems: 'center',
                      gap: 'var(--space-2)',
                      padding: '10px var(--space-3)',
                      border: isSelected ? '2px solid var(--color-action)' : '1px solid var(--color-border)',
                      borderRadius: 'var(--input-radius)',
                      background: isSelected ? 'var(--color-surface-raised)' : 'transparent',
                    }}
                  >
                    <span aria-hidden="true" style={{ fontSize: '15px', width: '18px', textAlign: 'center' }}>
                      {s.icon}
                    </span>
                    <span className="t-ui">
                      <span>{s.label}</span>
                    </span>
                  </button>
                );
              })}
            </div>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
            <span className="t-label" style={{ color: 'var(--color-text-subtle)' }}>
              <span>{t('appearance.theme')}</span>
            </span>
            <div
              style={{
                display: 'grid',
                gridTemplateColumns: 'repeat(auto-fill, minmax(160px, 1fr))',
                gap: 'var(--space-2)',
              }}
            >
              {THEMES.map((theme) => {
                const isSelected = appearance.theme === theme.id;
                const prev = THEME_PREVIEWS[theme.id] ?? {
                  bg: '#000',
                  text: '#fff',
                  action: '#fff',
                  font: 'sans-serif',
                };
                return (
                  <button
                    key={theme.id}
                    type="button"
                    onClick={() => handleThemePick(theme.id)}
                    role="radio"
                    aria-checked={isSelected}
                    style={{
                      boxSizing: 'border-box',
                      cursor: 'pointer',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: '6px',
                      padding: '6px',
                      border: isSelected ? '2px solid var(--color-action)' : '1px solid var(--color-border)',
                      borderRadius: 'var(--card-radius)',
                      background: 'var(--color-surface-raised)',
                    }}
                  >
                    <span
                      style={{
                        display: 'flex',
                        alignItems: 'flex-end',
                        justifyContent: 'space-between',
                        height: '52px',
                        padding: '8px 10px',
                        boxSizing: 'border-box',
                        borderRadius: 'calc(var(--card-radius) - 2px)',
                        background: prev.bg,
                        color: prev.text,
                        fontFamily: prev.font,
                        border: '1px solid var(--color-border)',
                      }}
                    >
                      <span style={{ fontSize: '20px', fontWeight: 600, lineHeight: 1 }}>
                        {SAMPLE_AA}
                      </span>
                      <span
                        style={{
                          width: '22px',
                          height: '10px',
                          borderRadius: '3px',
                          background: prev.action,
                        }}
                      />
                    </span>
                    <span
                      style={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        alignItems: 'center',
                        padding: '0 4px',
                      }}
                    >
                      <span style={{ display: 'flex', flexDirection: 'column', textAlign: 'left' }}>
                        <span className="t-ui">
                          <span>{theme.name}</span>
                        </span>
                        <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
                          <span>{theme.note}</span>
                        </span>
                      </span>
                      {isSelected && (
                        <span style={{ color: 'var(--color-action)' }}>
                          {CHECKMARK}
                        </span>
                      )}
                    </span>
                  </button>
                );
              })}
            </div>
          </div>

          {activeAccents.length > 0 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
                <span className="t-label" style={{ color: 'var(--color-text-subtle)' }}>
                  <span>{t('appearance.accent')}</span>
                </span>
                <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
                  <span>{currentThemeName}</span>
                </span>
              </div>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
                {activeAccents.map((accent) => {
                  const isSelected = appearance.accent === accent;
                  const color = ACCENT_COLORS[accent] ?? 'var(--color-action)';
                  return (
                    <button
                      key={accent}
                      type="button"
                      onClick={() => handleAccentPick(accent)}
                      role="radio"
                      aria-checked={isSelected}
                      style={{
                        boxSizing: 'border-box',
                        cursor: 'pointer',
                        display: 'flex',
                        alignItems: 'center',
                        gap: '8px',
                        padding: '6px 12px 6px 6px',
                        border: isSelected ? '2px solid var(--color-action)' : '1px solid var(--color-border)',
                        borderRadius: '999px',
                        background: 'transparent',
                      }}
                    >
                      <span
                        style={{
                          display: 'grid',
                          placeItems: 'center',
                          width: '22px',
                          height: '22px',
                          borderRadius: '999px',
                          background: color,
                          color: '#ffffff',
                          fontSize: '11px',
                        }}
                      >
                        {isSelected && CHECKMARK}
                      </span>
                      <span className="t-ui">
                        <span>{accent}</span>
                      </span>
                    </button>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
