import { en } from './en';

export type MessageCatalog = typeof en;
export type MessageKey = keyof MessageCatalog;

const pluralRules = new Intl.PluralRules('en');

export function translate(
  key: string,
  vars?: Record<string, string | number>,
): string {
  let templateKey = key;
  if (vars && typeof vars.count === 'number') {
    const rule = pluralRules.select(vars.count);
    const candidate = `${key}.${rule}`;
    if (candidate in en) {
      templateKey = candidate;
    }
  }

  const message: string = (en as Record<string, string>)[templateKey] ?? key;
  if (!vars) {
    return message;
  }

  return message.replace(/\{(\w+)\}/g, (match, varName) => {
    const val = vars[varName];
    return val !== undefined ? String(val) : match;
  });
}

export function useT() {
  return (key: MessageKey | (string & {}), vars?: Record<string, string | number>): string => {
    return translate(key, vars);
  };
}

export { en };
