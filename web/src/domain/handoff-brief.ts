export interface HandoffSource {
  id: string;
  skills: string[];
}

export interface HandoffBriefInput {
  sources: HandoffSource[];
}

// The curator agent follows the distill-lab skill: it reads a skill, scans one of its
// learning sources and records lessons in the skill's .meta/distill.yaml.
export function buildHandoffBrief(input: HandoffBriefInput): string {
  const lines = input.sources.map((source) =>
    source.skills.length > 0
      ? `- Distill ${source.skills.join(', ')} from source ${source.id}`
      : `- Source ${source.id} is linked to no skill: ask me which skill it should teach, then distill that skill from it`,
  );

  return `Use the distill-lab skill to learn from the sources below for the Skill Hub skills that follow them.

${lines.join('\n')}

For each line:
1. Read the skill first, then scan the source from its saved cursor to its current commit.
2. Record the lessons in the skill's .meta/distill.yaml with the distill-lab script (distill.py). Do not edit the skill itself and do not commit.
3. If a source cannot be read, name it and carry on with the others.
4. Finish with the lessons grouped by decision state, highest priority first, and ask me which to plan or reject.`;
}
