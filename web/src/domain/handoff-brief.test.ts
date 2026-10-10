import { describe, expect, it } from 'vitest';
import { buildHandoffBrief } from './handoff-brief';

describe('handoff-brief', () => {
  it('names each skill with its source and describes the distill-lab flow', () => {
    const brief = buildHandoffBrief({
      sources: [
        { id: 'source-a', skills: ['skill-1', 'skill-2'] },
        { id: 'source-b', skills: ['skill-3'] },
      ],
    });

    expect(brief).toContain('Use the distill-lab skill');
    expect(brief).toContain('- Distill skill-1, skill-2 from source source-a');
    expect(brief).toContain('- Distill skill-3 from source source-b');
    expect(brief).toContain('.meta/distill.yaml');
    expect(brief).toContain('distill.py');
    expect(brief).toContain('do not commit');
  });

  it('asks which skill to teach when a source is linked to none', () => {
    const brief = buildHandoffBrief({ sources: [{ id: 'orphan', skills: [] }] });
    expect(brief).toContain('Source orphan is linked to no skill: ask me which skill it should teach');
  });

  it('does not name any run tool', () => {
    const brief = buildHandoffBrief({ sources: [{ id: 's', skills: ['k'] }] });
    expect(brief).not.toContain('curation_');
    expect(brief).not.toContain('idempotency');
  });
});
