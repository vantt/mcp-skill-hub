import { describe, expect, it } from 'vitest';
import { translate, useT } from './index';

describe('i18n catalog', () => {
  it('translates static keys', () => {
    expect(translate('nav.home')).toBe('Home');
    expect(translate('skills.title')).toBe('Skills');
  });

  it('replaces placeholders', () => {
    const t = useT();
    expect(t('home.unreachable_sources.one', { count: 1 })).toBe('1 unreachable source');
  });

  it('selects plural suffixes .one and .other based on count', () => {
    const t = useT();
    expect(t('home.unreachable_sources', { count: 1 })).toBe('1 unreachable source');
    expect(t('home.unreachable_sources', { count: 2 })).toBe('2 unreachable sources');
    expect(t('home.changed_sources', { count: 1 })).toBe('1 changed source');
    expect(t('home.changed_sources', { count: 5 })).toBe('5 changed sources');
  });

  it('returns missing key itself', () => {
    const t = useT();
    expect(t('nonexistent.key')).toBe('nonexistent.key');
    expect(t('nonexistent.{var}', { var: 'abc' })).toBe('nonexistent.abc');
  });
});
