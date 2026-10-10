// One-line explanations and examples for the routing fields, shared by the Create form and the Editor
// so both say the same thing about what a field is for.
export interface FieldHelp {
  hint: string;
  example: string;
}

export const ROUTING_HELP = {
  operations: {
    hint: 'The kind of work this skill helps with. Separate several with commas.',
    example: 'review, debug',
  },
  triggers: {
    hint: 'Phrases a user might say that should bring this skill up. Agents match requests against these.',
    example: 'prepare a release, cut a release branch',
  },
  notFor: {
    hint: 'What this skill should not be used for, and why. It stops agents from picking it for the wrong job.',
    example: 'one-off typo fixes, writing new features',
  },
  minScope: {
    hint: 'How big a job has to be before this skill is worth loading: one step, several steps, or a whole project.',
    example: 'multi_step',
  },
} satisfies Record<string, FieldHelp>;

export const SCOPE_OPTIONS = [
  { value: '', label: 'Select scope' },
  { value: 'single_step', label: 'single_step: one step' },
  { value: 'multi_step', label: 'multi_step: several steps' },
  { value: 'project', label: 'project: a whole project' },
];

export const DRAFT_NOTE =
  'Creating this makes a draft. A draft is not routed to agents yet; it is used only after you activate it.';
