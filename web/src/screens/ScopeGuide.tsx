import { SCOPE_OPTIONS } from './routing-hints';

// Spells out what each Min scope value means and marks the chosen one, so the select needs no guesswork.
export function ScopeGuide({ value }: { value: string }) {
  return (
    <ul className="app-scope-guide" aria-label="What each scope means">
      {SCOPE_OPTIONS.filter((option) => option.value).map((option) => (
        <li
          key={option.value}
          className={`app-scope-guide__item${option.value === value ? ' app-scope-guide__item--active' : ''}`}
          aria-current={option.value === value ? 'true' : undefined}
        >
          <code>{option.value}</code>
          <span>{option.detail}</span>
        </li>
      ))}
    </ul>
  );
}
