# Phase 3: Launch

## Context
[plan.md](plan.md). One focused launch beats five scattered posts. It starts only after the gate in the plan is met.

## Requirements
- **One story, told three ways.** The problem ("I copied the same skill folders between projects and machines and they
  drifted"), the idea (one Git repo you own, agents pull the right skill on demand over MCP, edit and tune it centrally),
  the proof (a 40 second demo and the two-minute quickstart). Not a feature list.
- **Show HN post** (title starting "Show HN:", a first comment from the author explaining motivation, what is verified,
  what is not, and asking a specific question). Read the Show HN rules first.
- **One Reddit post** in the most on-topic community that allows it, chosen after reading its rules (candidates: r/ClaudeAI,
  r/mcp, r/LocalLLaMA); a different angle from the HN post, not a copy.
- **One short thread or post** on the owner's usual network (X, LinkedIn or a forum), with the demo clip.
- **Optional long form:** one article (dev.to or a blog) on a design choice that is interesting by itself: Git as the database,
  one-time invites that cannot be reused, or how an agent picks one skill per task. It links to the repository at the end.
- **Disclosure** in every post: who made it, that it is early, which hosts are verified.
- **Timing:** a weekday, the owner reachable for 6 to 8 hours afterwards to answer. One platform per day so replies get attention.

## Files
`plans/reports/promote-launch-drafts-<date>.md` (new): the three post drafts, the first comment, answers to the ten
questions most likely to be asked (including "why not symlinks", "how is this different from <registry>", "is my data safe",
"what does it send over the network").

## Steps
1. Write the drafts; check every claim against the docs.
2. Dry run: someone who has not seen the project reads the Show HN draft and the README and says in one sentence what it is.
3. The owner posts, or tells me "post" for a specific item and platform (I will only post what the owner approved word for word).
4. During the first 48 hours: answer every comment, collect issues, fix quick wins, note all feedback in the report.

## Validation
The one-sentence description from the dry run matches the positioning. After launch, every question in the first 48 hours
received an answer.

## Risk and rollback
A post cannot be fully taken back. Hence the gate, the dry run and word-for-word approval. If the quickstart breaks during
the launch, say so openly in the thread, fix it and reply with the fix.
