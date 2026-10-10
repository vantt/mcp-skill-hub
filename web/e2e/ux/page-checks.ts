import type { Page } from '@playwright/test';
import type { RunningServer } from '../support/server';
import type { Flow } from './flows';
import type { UxHub } from './seed-states';

export interface FlowMetrics {
  overflowX: boolean;
  viewportHeight: number;
  pageHeight: number;
  primaryActions: Array<{ label: string; belowFold: boolean; disabled: boolean }>;
  clipped: Array<{ tag: string; text: string }>;
}

// Runs in the page. Reports layout facts that a screenshot makes hard to judge.
export function collectMetrics(): FlowMetrics {
  const vw = window.innerWidth;
  const vh = window.innerHeight;
  const main = document.getElementById('main');
  const visible = (el: Element) => {
    const r = el.getBoundingClientRect();
    const cs = getComputedStyle(el);
    return r.width > 0 && r.height > 0 && cs.visibility !== 'hidden' && cs.display !== 'none';
  };
  const text = (el: Element) => (el.textContent ?? '').trim().replace(/\s+/g, ' ').slice(0, 80);

  const primaryActions = [...document.querySelectorAll('.fg-btn--primary')]
    .filter(visible)
    .map((el) => ({
      label: text(el),
      belowFold: el.getBoundingClientRect().bottom > vh,
      disabled: (el as HTMLButtonElement).disabled === true,
    }));

  const clipped: Array<{ tag: string; text: string }> = [];
  for (const el of document.querySelectorAll('pre, code, button, a, td, th, h1, h2, h3, label, p, li, span')) {
    if (clipped.length >= 25 || !visible(el)) {
      continue;
    }
    const cs = getComputedStyle(el);
    const clips = cs.overflowX !== 'visible' || cs.textOverflow === 'ellipsis';
    const hiddenText = clips && el.clientWidth > 0 && el.scrollWidth > el.clientWidth + 1;
    const offscreen = el.getBoundingClientRect().right > vw + 1 && text(el) !== '';
    if (hiddenText || offscreen) {
      clipped.push({ tag: el.tagName.toLowerCase(), text: text(el) });
    }
  }

  return {
    overflowX: document.documentElement.scrollWidth > vw,
    viewportHeight: vh,
    pageHeight: (main?.scrollHeight ?? document.documentElement.scrollHeight) + (main?.getBoundingClientRect().top ?? 0),
    primaryActions,
    clipped,
  };
}

// Puts the page in the state a flow describes: sizes the viewport, opens the route
// (the session token was stored by an earlier visit to server.url) and runs the flow's
// own preparation. Used by the capture run and by the regression guards.
export async function openFlow(page: Page, server: RunningServer, hub: UxHub, flow: Flow, width: number, height: number) {
  await page.setViewportSize({ width, height });
  await page.goto(`${server.origin}${flow.route}`);
  await page.waitForLoadState('networkidle');
  await page.locator('h1').first().waitFor({ timeout: 10000 }).catch(() => undefined);
  if (flow.prepare) {
    await flow.prepare({ page, hub });
    await page.waitForTimeout(200);
  }
}
