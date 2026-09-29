# Architecture Review: Agent ↔ Skill Server Resolution

## Report metadata

- **Project:** Curated Skill Hub over MCP
- **Reference:** `docs/PRD.md`, especially sections 25–33 and 58–62
- **Date:** 2026-09-28
- **Reviewer model:** `gpt-6-astra`
- **Provider:** `openai-codex`
- **Session identifier:** `01a0e68b-c7a3-75ac-9226-ff9b05f23e7b`
- **Status:** Architecture proposal for comparison with independent agent reviews; not a finalized protocol
- **Scope:** Only the Agent → Skill Server communication and recommendation architecture
- **Evidence level:** Design reasoning and proposed experiments, not measured benchmark results

---

# Executive recommendation

**Keep routing centralized, but reject rich agent-generated Skill Resolution Context as the primary contract.**

Use an **evidence-first request**:

> A compact description of the current task, relevant execution facts, explicit constraints, and optional semantic hints.

The server should own taxonomy, retrieval, ranking, abstention, and clarification. The agent should own task context—not translate that context into the server’s ontology.

The distinction matters:

- “Inspect retry behavior in this Kafka consumer; do not redesign the service” preserves the actual routing decision.
- `intent=review, domain=architecture, topics=[kafka,retry]` can erase the decisive constraint and route to the wrong skill.

The baseline’s largest weakness is not insufficient structure. It is **premature semantic compression**.

The recommended architecture is:

```text
Agent task understanding
        +
Relevant observed execution facts
        +
Explicit user constraints
        ↓
Compact evidence-first request
        ↓
Server-owned hybrid router
  compatibility checks
  lexical retrieval
  metadata matching
  optional semantic retrieval
  calibrated ranking and abstention
        ↓
resolved / needs_context / no_skill
        ↓
Progressive skill loading
```

Use an LLM only when the request contains sufficient evidence, cheaper routing remains uncertain, and the expected improvement justifies the extra latency and cost.

---

# 1. First-principles analysis

## 1.1 What is the router actually optimizing?

A router needs information that distinguishes useful procedures, not a comprehensive description of the project.

Consider a repository containing Kafka, Kubernetes, Go, and PostgreSQL. The current task might be:

- explain a single SQL expression;
- investigate duplicate message processing;
- review an architecture proposal;
- fix a documentation typo.

Repository facts are almost identical. The appropriate skill—or absence of one—is not.

Therefore:

1. **Task-local evidence must dominate ambient repository signals.**
2. **User constraints must survive compression.**
3. **Missing context is different from difficult classification.**
4. **A relevant skill is not necessarily a beneficial skill.**
5. **Routing accuracy must include abstention, not just selecting the closest catalog item.**

The actual optimization target is:

> Expected task improvement from using a skill, minus routing latency, context cost, and workflow disruption.

Maximizing recommendation acceptance alone encourages over-routing.

A conceptual decision rule is:

```text
Recommend skill S only when:

expected task benefit from S
  − activation/context cost
  − routing delay
  − workflow disruption

is meaningfully better than proceeding without S.
```

This does not require a numerically perfect utility model in V1. It requires explicitly evaluating the no-skill alternative rather than assuming every task needs a catalog match.

## 1.2 Baseline assumptions to challenge

### Assumption: “The agent already understands the task.”

Often, but not always. Understanding changes after tool results and user corrections. Requests need scope and freshness, not just semantic labels.

The protocol should tolerate incomplete understanding and allow correction without accumulating stale context.

### Assumption: “Structured fields improve consistency.”

Only if their meanings are stable and mechanically grounded. Open-ended fields such as `domain` often add model-specific classification noise.

Claude, Codex, and Gemini may all produce valid JSON but encode the same task differently. Schema validity is not evidence of semantic consistency.

### Assumption: “Close candidate scores mean call an LLM.”

Close scores may indicate:

- missing evidence;
- redundant catalog entries;
- equally acceptable alternatives;
- an underpowered ranker;
- a genuinely subtle semantic distinction.

Only the last two are plausible reasons for an LLM fallback. An LLM cannot reliably recover information that was never transmitted.

### Assumption: “The server must always choose.”

The server should be authoritative about routing policy, but not infallible about task interpretation or execution safety.

Abstention, clarification, user override, and agent safety checks are necessary escape routes.

### Assumption: “Structured facts should always outweigh free text.”

Not universally. A repository’s Kafka dependency must not outweigh the explicit instruction to edit only documentation.

Evidence strength depends on scope, freshness, and relevance—not merely whether a value occupies a structured field.

---

# 2. Information crossing the boundary

## 2.1 Recommended information hierarchy

| Information | Treatment | Reason |
|---|---|---|
| Current task description | Required, short natural language | Preserves action, target, and constraints |
| Explicit user constraints | First-class | Negation and scope are easily lost |
| Active artifact | Include when relevant | Distinguishes local work from broad project work |
| Relevant observed facts | Include selectively | Strong evidence without ontology dependence |
| Current execution state | Include when it changes the next procedure | Distinguishes diagnosis, implementation, and validation |
| Small exact user excerpt | Optional | Preserves important wording |
| Agent-inferred semantic labels | Optional, advisory | Sometimes useful, never authoritative |
| Repository profile | Cached reference or compact facts | Avoids repeated transmission |
| Available capabilities | Include when execution suitability depends on them | Prevents unusable recommendations |

## 2.2 Required task description

Describe **the next substantive action**, not the entire conversation or project:

> Diagnose duplicate processing after consumer restarts; inspect retry and acknowledgment ordering. Do not change the service architecture.

This is better than either a full conversation or a collection of disconnected labels.

A short verbatim user request can serve as the task description when it is already sufficient. Do not summarize merely for uniformity.

The task description should preserve:

- what should happen next;
- the target of that action;
- relevant scope boundaries;
- the desired outcome;
- explicit exclusions.

It should not require the agent to supply a catalog taxonomy.

## 2.3 Raw task versus summary

There should not be a universal rule that raw task text is always forbidden or always required.

Recommended policy:

- **Short, self-contained user task:** transmit it directly after necessary redaction.
- **Long conversation:** transmit a compact current-task description.
- **Important wording or correction:** include a small exact excerpt when paraphrasing risks losing meaning.
- **Ambiguous request:** request a targeted clarification rather than transmitting the whole conversation.

“Raw task” and “full conversation” are not interchangeable. A one-sentence raw task may be smaller and more faithful than a generated structured summary.

## 2.4 Factual observations versus interpretations

Distinguish three evidence sources:

1. **User-stated:** desired outcome, exclusions, scope.
2. **Tool-observed:** file type, dependency declaration, test result.
3. **Agent-inferred:** suspected failure cause, workflow interpretation.

Examples:

| Statement | Classification | Routing treatment |
|---|---|---|
| Kafka client is declared in the active component | Observable fact | Relevant compatibility or ranking evidence |
| Restart test invokes the handler twice | Observable fact | Strong task-local evidence |
| The cause is missing idempotency | Hypothesis | Advisory, not a hard filter |
| Domain is distributed systems | Semantic label | Optional weak hint |
| Do not redesign the service | Explicit user constraint | Strong applicability constraint |

Agent-inferred labels must not independently exclude candidates.

Do not force the agent to populate unknown fields. Unknown context should remain unknown.

## 2.5 Active artifact and recent execution state

Useful active-artifact information includes:

- artifact kind;
- language or format;
- small relative path hint;
- whether the target is a diff, a complete file, a design document, or a test;
- active component rather than whole-repository technology inventory.

Useful execution-state information includes:

- the current action;
- a relevant recent failure;
- whether the agent is investigating, implementing, or validating;
- what has already been ruled out;
- a user correction that changes scope.

Do not transmit a full execution trace. Send only state that changes the applicable procedure.

---

# 3. Information that should not cross by default

Exclude:

- full conversation history;
- private reasoning or chain-of-thought;
- whole source files, diffs, or logs;
- secrets, credentials, personal data;
- absolute local paths;
- unrelated repository inventories;
- complete tool histories;
- agent-generated candidate rankings;
- exhaustive topic lists;
- embeddings produced by arbitrary clients.

Small sanitized excerpts may be requested when they distinguish a routing choice.

**Privacy is not guaranteed by replacing raw text with a summary.** Summaries can still leak confidential information. Apply minimization and redaction to both.

Arbitrary client-generated embeddings should not be part of the base protocol because they introduce embedding-model compatibility, versioning, and debugging problems. Centralize embedding generation if semantic retrieval is used.

Untrusted artifact text must remain data. Neither task excerpts nor skill metadata should be treated as instructions to the router’s LLM.

---

# 4. Who should interpret intent?

## 4.1 Server-owned catalog interpretation

**The server should perform catalog-specific interpretation.**

The agent inevitably interprets the task when describing it. The goal is not to eliminate that interpretation; it is to avoid requiring a second, catalog-oriented classification.

The agent should not need to know:

- whether “debugging” belongs under “implementation” or “diagnosis”;
- which domain taxonomy the server uses;
- whether a topic is a hard requirement or a ranking boost;
- how skill families are organized.

## 4.2 Avoid an unnecessary intermediate classification

The server does not need to generate a formal intent label before retrieval.

Avoid:

```text
task
  ↓
incorrect intent class
  ↓
restricted retrieval
  ↓
wrong skill
```

Prefer:

```text
task + facts + constraints
  ↓
broad candidate retrieval
  ↓
compatibility checks and ranking
  ↓
resolution, clarification, or abstention
```

Intent labels may be useful internally for explanations or features, but they should not become an irreversible routing bottleneck.

## 4.3 Interpretation without an LLM on every request

Use:

1. Normalized synonyms and aliases.
2. Lexical retrieval over descriptions, activation examples, and exclusions.
3. Structured compatibility checks.
4. Optional compact embedding retrieval.
5. A lightweight ranker over interpretable features.
6. Calibrated abstention and clarification thresholds.

Semantic retrieval can help, but embeddings are not a prerequisite for the first prototype.

Skill metadata should include not just descriptions, but examples of:

- when the skill is useful;
- when it is not useful;
- the expected artifact;
- required capabilities;
- typical scope;
- adjacent skills with different applicability.

These are routing assets the server can improve centrally.

---

# 5. Cross-model consistency

Rich SRC generation is unlikely to be equivalent across models without significant adapter and evaluation work.

Potential failures include:

- different labels for equivalent tasks;
- omitted exclusions;
- different levels of abstraction;
- excessive inferred topics;
- confusing project domain with current task domain;
- treating a hypothesis as an established fact.

Even evidence-first requests can vary: models select different facts and compress differently.

Reduce variance through:

- a small schema;
- examples of good task descriptions;
- mechanically populated artifact fields;
- optional rather than compulsory semantic labels;
- preservation of explicit constraints;
- cross-model conformance tests;
- clear behavior for unknown fields.

A successful protocol should remain useful when a client provides only a good task description. Richer context should improve accuracy, not be mandatory for basic operation.

---

# 6. Preferred architecture and responsibilities

## 6.1 Agent responsibility

The agent should:

- describe the current operation accurately;
- preserve explicit user constraints;
- provide relevant local facts unavailable to the server;
- distinguish observations from hypotheses;
- answer targeted clarification from existing context when possible;
- sanity-check applicability and safety;
- load only the selected skill and needed supporting resources;
- report material context changes.

The agent should not:

- enumerate the catalog;
- maintain catalog taxonomy;
- rank a long candidate list;
- generate authoritative routing labels;
- send its entire reasoning history.

## 6.2 Server responsibility

The server should:

- own skill taxonomy and applicability metadata;
- normalize requests;
- retrieve and rank candidates;
- check compatibility and permissions;
- decide whether a skill is worth using;
- detect uncertainty and ask discriminating questions;
- own rerouting and supporting-skill selection;
- version routing policy;
- record reproducible decisions where policy permits.

## 6.3 Optional client adapter responsibility

A local adapter may supply environment facts automatically.

This must remain optional. MCP clients differ in what they expose; the protocol must work without privileged filesystem access.

The adapter should:

- collect only relevant facts;
- scope facts to the active component;
- attach provenance and freshness information where practical;
- redact sensitive information;
- avoid repeated transmission of unchanged context.

---

# 7. Server routing pipeline

```text
Validate and minimize request
          ↓
Resolve authorized cached context
          ↓
Check explicit exclusions and capability constraints
          ↓
Retrieve candidates through parallel channels
  lexical
  metadata
  optional semantic
  soft family routing
          ↓
Score applicability and expected usefulness
          ↓
Check uncertainty, conflicts, redundancy, and coverage
          ↓
resolve / clarify / abstain / bounded LLM fallback
```

## 7.1 Deterministic versus semantic routing

The recommended design is hybrid, with a deterministic normal path where feasible.

“Deterministic” and “retrieval-based” are not opposites. Lexical retrieval and fixed ranking rules can be deterministic for a given request, catalog revision, and policy version.

The useful questions are:

- Is the decision reproducible?
- Is it accurate?
- Does it generalize beyond exact vocabulary?
- Can its uncertainty be detected?

## 7.2 Hard rules

Use hard rules sparingly for:

- authorization;
- disabled or unavailable skills;
- execution requirements;
- explicit incompatibilities.

Use soft boosts for most technology and topic matches.

Overly aggressive hard filtering can eliminate the correct skill because of incomplete context or imperfect metadata.

## 7.3 Hierarchical routing

Hierarchy is useful as a retrieval accelerator, **not an irreversible first-stage classifier**.

Search plausible families in parallel and retain a global fallback.

Otherwise:

```text
wrong first-stage family
  ↓
correct skill becomes unreachable
```

A hierarchical architecture should be justified by measured latency or retrieval-quality gains, not adopted solely because the catalog is large.

---

# 8. Proposed request schema

The following is an illustrative V1 contract, not a finalized JSON Schema.

```json
{
  "schema_version": "1",
  "request_id": "req_101",
  "task": {
    "description": "Diagnose duplicate processing after consumer restarts.",
    "constraints": [
      "Focus on retry and acknowledgment ordering.",
      "Do not redesign the service."
    ]
  },
  "context": {
    "active_artifact": {
      "kind": "source-code",
      "language": "go",
      "path_hint": "consumer/handler.go"
    },
    "facts": [
      {
        "key": "dependency",
        "value": "kafka",
        "basis": "tool",
        "scope": "active-component"
      },
      {
        "key": "test-result",
        "value": "Handler invoked twice after simulated restart.",
        "basis": "tool",
        "scope": "current-task"
      }
    ],
    "execution": {
      "current_action": "Investigating the failing restart test.",
      "available_capabilities": ["read-files", "run-tests"]
    },
    "environment_ref": "env_42"
  },
  "hints": {
    "suspected_issue": "Retry and acknowledgment interaction."
  },
  "preferences": {
    "latency_class": "interactive"
  }
}
```

## 8.1 Contract rules

- Only the task description needs to be mandatory for an initial request.
- Paths are optional, relative, and minimized.
- `facts` uses a small extensible vocabulary; it must not become a giant ontology.
- Hints cannot independently exclude candidates.
- Environment references must be authorized, versioned, and checked for staleness.
- Tool-derived provenance is helpful but not inherently trusted across arbitrary clients.
- Server policy sets budgets; client preferences cannot authorize unbounded inference.
- Enforce byte/token limits and explicit truncation behavior.
- Do not silently discard explicit constraints when a request exceeds its budget.

A reasonable prototype target is **roughly 150–400 request tokens**, not a fixed requirement. Measure whether additional context actually improves decisions.

## 8.2 Why semantic fields are optional

An agent may already know a useful hypothesis or intent. Discarding that information entirely is unnecessary.

However, fields such as `intent`, `domain`, and `topics` should be advisory hints rather than required routing keys.

The core request should remain useful if every semantic hint is absent.

---

# 9. Proposed response schema

## 9.1 Resolved response

```json
{
  "status": "resolved",
  "resolution_id": "res_101",
  "context_revision": 1,
  "primary": {
    "id": "message-processing-debug",
    "version": "v17",
    "applicability": "Diagnose retry, acknowledgment, and duplicate-processing behavior."
  },
  "supporting": [],
  "confidence": {
    "band": "high",
    "calibration_version": "routing-eval-4"
  },
  "reason_codes": [
    "task_match",
    "active_component_match"
  ],
  "catalog_revision": "catalog_92",
  "policy_version": "router_8"
}
```

The short applicability sentence lets the agent detect obvious mismatch without reading a shortlist or requesting a lengthy explanation.

The response should return stable skill identity and sufficient version information to avoid resolving against one version and unknowingly loading another.

## 9.2 Confidence handling

Do not expose a normalized retrieval score as a probability.

Separate internally:

- strength of applicability evidence;
- uncertainty between distinct workflows;
- expected incremental usefulness;
- whether required context is missing;
- catalog coverage uncertainty.

A score margin alone is insufficient:

- Two nearly identical good skills can tie harmlessly.
- One irrelevant skill can win by a large margin.
- High relevance may still have negligible utility.

Use calibrated confidence bands initially. Expose probabilities only after defining and evaluating their meaning.

Confidence should describe an evaluated decision property, not an LLM’s subjective confidence or the ranker’s raw score.

---

# 10. Ambiguity detection and clarification

## 10.1 Detecting ambiguity

Useful signals include:

- plausible candidates require materially different workflows;
- distinguishing facts are absent;
- task text and observations conflict;
- retrieval channels disagree;
- all candidates have weak applicability;
- the request resembles known out-of-distribution cases;
- several candidates are near-duplicates.

Only some of these need a question.

Equivalent skills can be resolved through a stable server preference. Catalog duplication needs curation. Weak overall fit may warrant abstention rather than clarification.

## 10.2 Preferred `needs_context` protocol

Ask **one high-value question**, tied to a decision:

```json
{
  "status": "needs_context",
  "resolution_id": "res_202",
  "context_revision": 1,
  "question": {
    "id": "review_focus",
    "text": "Is this review focused on correctness, security, or both?",
    "choices": ["correctness", "security", "both", "unknown"],
    "answer_from": "existing_context_first"
  }
}
```

Important properties:

- bounded answers, with a free-text escape when necessary;
- an `unknown` option;
- no requirement to know the server’s taxonomy;
- no candidate list disguised as a question;
- no user interruption if the agent already knows;
- a small clarification budget, such as one normal clarification round.

Ask only when expected decision improvement exceeds interruption cost.

## 10.3 Clarification answers

A follow-up should identify:

- the resolution;
- the context revision;
- the question being answered;
- the answer;
- whether it came from existing context or the user.

Example:

```json
{
  "schema_version": "1",
  "request_id": "req_203",
  "previous_resolution_id": "res_202",
  "expected_context_revision": 1,
  "clarification": {
    "question_id": "review_focus",
    "answer": "security",
    "basis": "user"
  }
}
```

This answer structure is illustrative. The important property is explicit association with a specific unresolved question, rather than an unstructured conversation with the router.

---

# 11. `no_skill` and prevention of over-routing

## 11.1 When to return `no_skill`

Return it when:

- no applicable skill exists;
- the task is too simple to benefit;
- available skills conflict with constraints;
- available skills require unavailable capabilities;
- remaining uncertainty makes recommendation unsafe or unhelpful.

Distinguish reasons:

```json
{
  "status": "no_skill",
  "resolution_id": "res_303",
  "reason_code": "low_expected_benefit",
  "retry_when": "task_scope_changes"
}
```

Other useful reason categories include insufficient catalog coverage, incompatible constraints, and unresolved ambiguity.

An infrastructure failure is an error—not `no_skill`.

## 11.2 Two separate gates

There must be two gates:

1. **Should the agent consult the router?**
2. **Should the router recommend a skill?**

The first can be a small catalog-independent policy:

> Consult for new substantive work, not every edit, sentence, or tool call.

Use accepted resolutions across the same task scope. Reconsult after material changes, not every new observation.

A prior `no_skill` should also suppress repeated calls until relevant context changes.

## 11.3 Avoiding mandatory skill usage

The success criterion is not that every task receives a skill.

A router that declines trivial or poorly supported activation may be better than one with higher coverage but more irrelevant instructions.

Benchmark against ordinary execution without skills. Otherwise the system cannot determine whether it improves outcomes or merely adds procedure.

---

# 12. Interaction sequences

## 12.1 Normal path

```text
Agent identifies substantive task
  → adapter supplies relevant cached facts
  → skill_resolve
  → server retrieves, scores, and resolves
  → agent checks short applicability description
  → skills/get
  → resources/read(SKILL.md)
  → supporting files only as needed
```

The sanity check is not independent reranking.

The agent checks for obvious scope mismatch, unavailable capabilities, user conflicts, or safety issues. It does not compare the selected skill against a hidden imagined catalog.

## 12.2 Ambiguous path

```text
skill_resolve
  → server identifies missing discriminator
  → needs_context
  → agent answers from known context
       or asks user if materially necessary
  → server resolves or abstains
```

Avoid an LLM call when the missing information is simply unavailable.

## 12.3 Re-resolution path

```json
{
  "schema_version": "1",
  "request_id": "req_102",
  "previous_resolution_id": "res_101",
  "expected_context_revision": 1,
  "feedback": {
    "kind": "scope_mismatch"
  },
  "context_patch": {
    "task.constraints": {
      "replace": [
        "Review only the SQL transaction boundary.",
        "Kafka behavior is outside scope."
      ]
    }
  }
}
```

Define patch operations formally; support deletion as well as addition. Otherwise stale context accumulates and biases future recommendations.

The server should avoid returning the rejected skill unchanged unless new evidence justifies it.

Limit repeated correction loops and eventually abstain with a clear explanation.

Resolution state may expire. If it does, request a fresh compact snapshot rather than pretending the patch was applied.

Use context revisions to prevent a delayed response or patch from silently overwriting newer task context.

## 12.4 Can the agent select another candidate?

**Not on the normal path.** Prefer veto plus corrected context and re-resolution.

However, this should not become an absolute restriction on user authority:

- An explicit user choice should be honored, subject to safety and access rules.
- An agent may decline an unsafe or incompatible procedure.
- If the server is unavailable, ordinary task execution should remain possible.

These are not reasons to expose a routine candidate buffet.

An agent-selected-shortlist architecture remains a valid experimental challenger. If it consistently improves outcomes by using private context, that is evidence the boundary contract or authority policy needs revision.

---

# 13. LLM fallback policy

Use a server-side LLM only when all are true:

1. Retrieved candidates are plausibly applicable.
2. The request contains enough evidence to distinguish them.
3. Cheaper ranking remains uncertain.
4. Expected benefit justifies latency and cost.
5. Privacy and deployment policy permit the call.

## 13.1 Suitable cases

- nuanced constraints;
- unusual terminology;
- genuinely cross-domain procedure selection;
- distinctions not captured by current lexical or structured ranking features.

## 13.2 Unsuitable cases

- missing user preference;
- stale environment facts;
- no catalog coverage;
- duplicate skills;
- trivial tasks.

## 13.3 Bounded fallback contract

Send only:

- the minimized request;
- a small candidate set;
- compact applicability descriptions;
- necessary constraints and compatibility facts.

Do not send entire skills by default.

Require structured output with abstention. Bound:

- candidate count;
- input and output tokens;
- execution time;
- retries.

An LLM’s self-reported confidence is not calibrated confidence.

Measure the fallback’s incremental accuracy relative to its extra cost. If fallback is frequently required, investigate metadata and request quality before assuming a larger model is the solution.

---

# 14. Skill composition policy

**V1 should return one primary skill by default.**

Supporting skills should be returned only when:

- they add a distinct necessary procedure;
- they are compatible with the primary;
- their applicability is independently supported;
- the relationship is curated or explicitly validated.

“Complements” is not enough to justify automatic loading.

Distinguish:

1. **Required supporting procedure.**
2. **Conditionally useful follow-up.**

Only the former belongs in immediate activation. Prefer progressive activation for the latter.

Do not generate a synthesized execution package in V1.

Composition introduces additional problems:

- ordering;
- conflicting instructions;
- duplicated steps;
- resource budgets;
- provenance;
- attribution of task outcomes;
- evaluating the combined procedure.

Those are not solved merely by finding several relevant skills.

---

# 15. Example requests and responses

These examples are abbreviated instances of the proposed contract.

## 15.1 Runtime debugging: resolve immediately

### Request

```json
{
  "schema_version": "1",
  "request_id": "r_debug",
  "task": {
    "description": "Diagnose duplicate message handling after consumer restart.",
    "constraints": ["No architecture redesign."]
  },
  "context": {
    "active_artifact": {
      "kind": "source-code",
      "language": "go"
    },
    "facts": [
      {
        "key": "dependency",
        "value": "kafka",
        "basis": "tool",
        "scope": "active-component"
      }
    ]
  }
}
```

### Response

```json
{
  "status": "resolved",
  "resolution_id": "s_debug",
  "primary": {
    "id": "message-processing-debug",
    "applicability": "Investigate retry, acknowledgment, and duplicate-processing failures."
  },
  "supporting": [],
  "confidence": {"band": "high"},
  "reason_codes": ["task_match", "technology_match"]
}
```

### Why this should work

The task description carries the procedure and scope. Kafka is corroborating context, not the entire basis for selection.

The exclusion prevents the router from confusing a narrow runtime investigation with broad architecture review.

## 15.2 Research synthesis: clarify a consequential distinction

### Request

```json
{
  "schema_version": "1",
  "request_id": "r_research",
  "task": {
    "description": "Compare these studies and prepare a recommendation for hospital leadership."
  },
  "context": {
    "active_artifact": {
      "kind": "research-papers"
    }
  }
}
```

### Response

```json
{
  "status": "needs_context",
  "resolution_id": "s_research",
  "question": {
    "id": "decision_type",
    "text": "Is the recommendation about clinical effectiveness or implementation feasibility?",
    "choices": [
      "clinical-effectiveness",
      "implementation-feasibility",
      "both",
      "unknown"
    ],
    "answer_from": "existing_context_first"
  }
}
```

### Why clarification is appropriate

The missing distinction changes the procedure. Evidence appraisal and implementation feasibility are different tasks even when the same papers are involved.

After “clinical effectiveness,” the server can resolve an evidence-appraisal skill. It should not infer the answer merely from “hospital.”

## 15.3 Small editorial change: abstain

### Request

```json
{
  "schema_version": "1",
  "request_id": "r_edit",
  "task": {
    "description": "Change 'recieve' to 'receive' in the README heading.",
    "constraints": ["Make no other changes."]
  },
  "context": {
    "active_artifact": {
      "kind": "documentation"
    }
  }
}
```

### Response

```json
{
  "status": "no_skill",
  "resolution_id": "s_edit",
  "reason_code": "low_expected_benefit",
  "retry_when": "task_scope_changes"
}
```

### Why abstention is correct

A writing or documentation skill may be topically relevant, but activation adds little value and risks expanding the requested change.

Ideally the agent’s consultation gate avoids this call entirely. The server must still handle it correctly.

---

# 16. Architectural alternatives

The following comparisons are hypotheses to test, not measured rankings.

## 16.1 Accuracy and operating cost

| Alternative | Accuracy | Cross-model consistency | Latency | Token cost | Privacy |
|---|---|---|---|---|---|
| **A. Rich agent SRC** | Good when labels are correct; vulnerable to lost constraints | Low–medium | Low server latency, extra agent work | Moderate structured overhead | Usually compact; inferred labels can still disclose sensitive context |
| **B. Facts + compact task** | Strong expected balance; preserves nuance | Medium–high with adapters | Low on normal path | Low–moderate | Strong minimization possible |
| **C. Raw task + server interpretation** | Strong for self-contained tasks; weak when execution context is omitted | High for identical inputs, not necessarily identical model-selected excerpts | High if LLM-dependent | Potentially high | Greatest disclosure risk when raw context expands |
| **D. Deterministic hierarchical router** | Strong in stable taxonomy; brittle at boundaries | High | Very low | Low | Good |
| **E. Server shortlist + agent selection** | Can exploit private context; can also amplify selection bias | Low–medium | Extra selection work | Higher response/context cost | Private facts may remain local, but request privacy is otherwise unchanged |

## 16.2 Engineering characteristics

| Alternative | Complexity | Debuggability | Central improvement | Agent/server coupling |
|---|---|---|---|---|
| **A** | Ontology and client conformance are expensive | Clear fields, unclear reasons for mislabeling | Limited by client-produced labels | High semantic coupling |
| **B** | Moderate; fact selection and calibration need care | Good evidence-to-decision tracing | Strong | Low–moderate |
| **C** | Simple client, costly server operations | Harder when interpretation is opaque | Strong | Low schema coupling |
| **D** | Simple initially; rule growth becomes costly | Excellent until rule interactions proliferate | Strong but labor-intensive | Low if classification stays server-side |
| **E** | Server simpler; distributed behavior harder to evaluate | Final decision spread across components | Partial | High behavioral coupling |

## 16.3 Alternative A: rich agent-generated SRC

### Strengths

- Compact if the ontology is stable.
- Easy to inspect structurally.
- Can make deterministic filtering straightforward.
- Reuses the agent’s task understanding.

### Weaknesses

- Requires every client to interpret fields consistently.
- Loses nuance when tasks do not fit the taxonomy.
- Encourages unjustified semantic certainty.
- Couples client prompts to evolving server metadata.
- Often preserves nouns while losing scope and negation.

### Verdict

Useful as optional hints, not as the sole or primary contract.

## 16.4 Alternative B: facts plus compact task description

### Strengths

- Preserves natural-language nuance.
- Centralizes catalog interpretation.
- Supports mechanically populated context.
- Works with sparse requests.
- Can improve centrally without changing every agent.

### Weaknesses

- Summarization can still omit critical information.
- Fact selection needs discipline.
- Requires good metadata and abstention calibration.

### Verdict

Preferred base architecture.

## 16.5 Alternative C: raw task and full server interpretation

### Strengths

- Minimal client-side routing logic.
- Potentially preserves exact user wording.
- Centralizes interpretation.

### Weaknesses

- Raw task alone omits execution context.
- Full context creates privacy and token costs.
- LLM-based interpretation can duplicate agent reasoning.
- Higher tail latency if interpretation is always model-dependent.

### Verdict

A short raw task is a valid input. Full server-side reinterpretation should not be the universal normal path.

## 16.6 Alternative D: deterministic hierarchical router

### Strengths

- Fast and reproducible.
- Easy to debug with a small stable taxonomy.
- Low token and inference costs.

### Weaknesses

- Early classification errors can be unrecoverable.
- Cross-domain tasks fit poorly.
- Rule interactions become complex as the catalog grows.

### Verdict

Use as a soft server-side optimization with global fallback, not a hard protocol requirement.

## 16.7 Alternative E: shortlist with agent final selection

### Strengths

- Uses private context without transmitting all of it.
- May handle nuanced task understanding better.
- Can reveal limitations of server-only routing.

### Weaknesses

- Reintroduces distributed routing intelligence.
- Increases response tokens and agent reasoning.
- Creates model-specific selection behavior.
- Makes feedback attribution harder.

### Verdict

Not the preferred normal path, but an important benchmark challenger.

If private context repeatedly makes agent selection superior, that is evidence against an overly strict server-only policy—not something to dismiss philosophically.

## 16.8 Preferred combination

**B, with D as a soft server-side optimization and bounded LLM assistance.**

---

# 17. Automatically derived environment context

A client adapter can obtain:

- active file language and artifact type;
- selected diff or test target;
- package manifests and dependency presence;
- repository/worktree revision;
- available tools and permissions;
- recent test exit status;
- presence of relevant schemas or configuration.

Automatic collection needs relevance and freshness controls.

Do not send every dependency in a monorepo. Prefer active-component scope.

Environment fingerprints should change when relevant inputs change, not merely on every unrelated repository commit.

The server cannot automatically obtain:

- unstated user priorities;
- corrections in the conversation;
- why the agent is currently examining a file;
- whether a workaround or root-cause fix is desired.

Those belong in the compact task description and constraints.

A server-hosted deployment must not assume access to the agent’s local filesystem. A local deployment should still use explicit authorization and minimized collection rather than treating proximity as permission.

---

# 18. Feedback, learning, and Git-first persistence

Separate operational telemetry from durable learned behavior.

## 18.1 Disposable runtime telemetry

May include, under retention and privacy policy:

- latency and stage timings;
- candidate scores;
- retrieval channels;
- clarification frequency;
- vetoes and reroutes;
- cache behavior.

Losing this data must not silently alter the current routing policy.

## 18.2 Durable, versioned artifacts

Anything required to reproduce learned routing behavior should have a Git representation:

- sanitized labeled evaluation cases;
- skill activation examples and exclusions;
- ranking configuration;
- calibrated thresholds;
- approved relationship changes;
- model parameters, or reproducible training inputs/configuration plus immutable artifact identification.

If external model artifacts are used, their availability must not undermine the repository’s rebuild guarantee. Prefer small reproducible routing models or explicit artifact-retention policy rather than a pointer to something that may disappear.

Do not allow online learning to change durable routing behavior only inside SQLite.

## 18.3 Feedback semantics

Persisted feedback should distinguish:

- accepted recommendation;
- actually loaded skill;
- useful outcome;
- explicit mismatch;
- user-selected override.

Acceptance is a weak proxy for usefulness. Agents may accept a bad recommendation because the protocol tells them to.

Do not commit raw conversations or sensitive tool output merely to satisfy Git-first requirements.

A practical workflow is:

```text
runtime telemetry
  ↓
privacy-aware review or aggregation
  ↓
approved evaluation examples / policy changes
  ↓
Git-visible durable artifacts
  ↓
rebuildable runtime ranking state
```

---

# 19. Metrics that reveal a bad routing architecture

Measure by task category and by client model:

- applicable-skill selection rate, allowing multiple valid answers;
- false-positive routing on no-skill tasks;
- missed useful skills;
- downstream task quality against a no-skill baseline;
- cross-model disagreement on equivalent execution states;
- correction and repeated-veto rates;
- user clarification rate;
- unnecessary clarification rate;
- p50/p95 end-to-end latency;
- total routing tokens, including agent request construction;
- LLM fallback frequency and incremental benefit;
- confidence calibration;
- sensitivity to irrelevant repository facts;
- stability under paraphrases;
- stability under catalog growth.

A crucial warning sign:

> High recommendation acceptance with no measurable task improvement.

Additional warning signs:

- adding irrelevant technology facts changes the recommendation;
- different models repeatedly choose different domains for equivalent tasks;
- the server asks questions already answered in the request;
- a large fraction of requests require reranking by an LLM;
- re-resolution returns the same rejected skill without addressing the correction;
- the router chooses a skill for nearly every task;
- adding near-duplicate catalog entries changes confidence dramatically.

Track stage-level latency, but optimize end-to-end cost. Moving reasoning from the server into a longer agent-generated SRC does not make that reasoning free.

---

# 20. Most likely failure modes at scale

| Failure mode | Consequence | Mitigation direction |
|---|---|---|
| Near-duplicate skills | Unstable winners and misleading score margins | Canonical preferences, duplicate curation, equivalence-aware evaluation |
| Metadata inconsistency | Discovery depends on author vocabulary | Activation examples, normalization, metadata linting |
| Ambient-context domination | Repository technologies overwhelm current task | Scope-aware features and explicit task priority |
| Constraint loss | “Only,” “do not,” and “without” disappear | First-class constraints and paraphrase tests |
| Stale context | Cached facts survive task or branch changes | Revisioned references and replacement/deletion semantics |
| Hierarchy lockout | Correct skill becomes unreachable | Multi-family retrieval and global fallback |
| Clarification loops | Agent and server restate ambiguity repeatedly | Question identifiers, bounded rounds, abstention |
| False confidence | Scores become unreliable as catalog changes | Recalibration and coverage-aware evaluation |
| Feedback bias | Accepted recommendations reinforce themselves | Outcome labels and no-skill baseline |
| Prompt injection | Untrusted text manipulates LLM router | Treat excerpts and metadata as data; bounded structured fallback |
| Composition explosion | Relevant skills create contradictory procedures | Primary-only default and validated composition |
| Catalog gaps disguised as certainty | Nearest skill is treated as correct | Absolute applicability thresholds and abstention |

Curated routing metadata reduces prompt-injection risk but does not eliminate it.

---

# 21. Prototype plan before freezing the protocol

## 21.1 Build the evaluation set first

Construct representative cases containing:

- task description;
- known execution context;
- explicit constraints;
- acceptable skills or acceptable skill sets;
- whether no-skill is appropriate;
- whether clarification is genuinely necessary;
- the critical evidence needed to decide.

Include:

- ambiguous cases;
- explicit no-skill cases;
- negation and narrow-scope constraints;
- misleading repository signals;
- overlapping skills;
- cross-domain tasks;
- stale-context corrections;
- tasks outside catalog coverage.

Allow multiple valid recommendations. A benchmark that assumes one exact skill ID for every task can punish equally useful alternatives and overstate routing instability.

## 21.2 Isolate boundary-design effects

Compare A, B, C, and E using the **same retrieval backend**.

Have Claude, Codex, and Gemini independently encode the same execution states.

Measure:

- information retained;
- information invented;
- routing agreement;
- downstream accuracy;
- request tokens;
- generation latency;
- correction needs.

Without a fixed backend, improvements could come from the retrieval implementation rather than the communication contract.

## 21.3 Add routing complexity incrementally

Start with:

```text
compact task + facts
  +
lexical retrieval
  +
applicability metadata
  +
explicit abstention
```

Then test separately:

1. Semantic retrieval.
2. Lightweight learned ranking.
3. Soft hierarchy.
4. Targeted clarification.
5. Bounded LLM fallback.
6. Supporting-skill selection.

Do not introduce every component simultaneously. That makes failures difficult to attribute.

## 21.4 Stress-test catalog growth

Evaluate the same tasks as the catalog expands and near-duplicates are introduced.

The protocol should not depend on a tiny, unusually clean initial catalog.

Test whether:

- correct skills remain retrievable;
- confidence remains meaningful;
- latency stays within budget;
- irrelevant skills increase false-positive routing;
- metadata normalization remains effective.

## 21.5 Evaluate actual task outcomes

Routing labels alone are insufficient.

For a subset of tasks, compare:

- no skill;
- server-selected skill;
- expert-selected skill;
- agent-selected shortlist winner.

This establishes whether recommendation errors matter and whether using skills improves execution at all.

---

# 22. Final conclusions

## 22.1 Recommended architecture

**Evidence-first, server-owned hybrid routing.**

- Required compact task description.
- Explicit constraints.
- Selective factual execution context.
- Optional, nonbinding semantic hints.
- Central retrieval, compatibility checks, ranking, and calibrated abstention.
- Targeted clarification before expensive reinterpretation.
- One primary skill by default.
- Re-resolution instead of routine agent reranking.
- Bounded LLM fallback only when sufficient evidence exists.

This retains the strongest part of the baseline—centralized routing—while rejecting the assumption that every agent should construct a rich semantic classification compatible with the server’s catalog.

## 22.2 Top three risks

1. **Task compression still loses decisive information.**
   - Evidence-first design reduces ontology coupling but does not eliminate lossy summarization.
   - Mitigation: preserve constraints, allow short exact excerpts, test adversarial paraphrases.

2. **Catalog metadata and abstention calibration become the real accuracy bottlenecks.**
   - Better request design cannot compensate for indistinguishable or poorly described skills.
   - Mitigation: activation examples, exclusions, duplicate management, no-skill evaluation.

3. **Fact collection and freshness vary across clients, undermining consistency.**
   - Some clients provide precise tool-derived context; others provide only model summaries.
   - Mitigation: sparse-request support, optional adapters, provenance, scoped caching, revision-aware updates.

## 22.3 What should be prototyped first?

Before freezing the schema:

1. Build a representative benchmark with real task and execution context.
2. Compare rich SRC, evidence-first requests, raw-task interpretation, and agent shortlist selection.
3. Use multiple frontier models to generate requests for identical cases.
4. Hold retrieval constant while evaluating the boundary contract.
5. Start with lexical retrieval and applicability metadata.
6. Measure abstention and downstream benefit, not only top-1 matching.

## 22.4 Evidence that would change this recommendation

Reconsider if:

- Rich SRC consistently outperforms evidence-first requests across models and catalog changes.
- Raw-task interpretation yields substantial downstream gains within acceptable privacy, cost, and latency budgets.
- Agent shortlist selection materially improves outcomes without excessive tokens or cross-model variance.
- Deterministic routing matches hybrid accuracy, including abstention and out-of-distribution cases.
- Most remaining errors arise from private context that cannot be compactly transmitted.

The architecture should remain falsifiable. Centralized authority is a means to consistency and maintainability, not an end in itself.

> **Select the protocol by end-to-end usefulness—not by whether its division of responsibilities looks architecturally elegant.**

---

# Appendix A. Coexistence of Local Skills and Hub-Provided Skills

## A.1 Architectural correction: recommendation is not activation authority

The original recommendation needs an explicit boundary:

> **The host owns activation policy. The Hub owns recommendation within delegated scope.**

Centralized catalog routing does not imply that the Hub controls every skill available to an agent.

An agent may simultaneously receive:

- project-local skills;
- user-level skills;
- client-integrated procedures;
- Hub-provided skills;
- project or organizational instructions;
- system/developer instructions and host-enforced permissions.

MCP distribution does not automatically expose local activation state to the Hub, nor give the Hub control over native client discovery.

Without explicit coordination, two independent routers may select competing procedures for the same operation.

This appendix refines the earlier recommendation rather than transferring catalog ranking back to the agent. The host arbitrates activation and constraints; the Hub continues to rank its catalog within the authorized scope.

## A.2 Conflict categories

| Conflict | Example | Risk |
|---|---|---|
| Functional overlap | Local and Hub both offer code review | Duplicate instructions and unnecessary context |
| Version drift | Local is an older copy of a Hub skill | Inconsistent behavior and misleading provenance |
| Procedure contradiction | One skill requires tests first; another requires refactoring first | Conflicting execution order |
| Scope mismatch | Local is repository-specific; Hub is generic | Loss of important local conventions |
| Double activation | Local skill is already loaded before Hub resolution | Two procedures directing one operation |
| Routing recursion | Local skill calls Hub; Hub skill asks for another resolution | Repeated calls and activation loops |
| Capability mismatch | Hub procedure assumes unavailable tools | Unexecutable workflow |
| Policy conflict | Procedure requests actions prohibited by host policy | Unauthorized execution |

Identical names do not prove identical identity. Different names do not prove functional independence.

The main risk is not two files existing. It is two incompatible procedures directing the same operation without an explicit coordination decision.

## A.3 Reject universal local-first or Hub-first precedence

Neither of these is a sound universal policy:

```text
Hub always overrides local
```

```text
Local always overrides Hub
```

A local skill may encode essential repository knowledge, but it can also be outdated or overly broad. A Hub skill may be better curated, but still violate local conventions.

**Storage location is not authority.**

Separate:

### Policy

Authorized constraints on what may happen:

- do not transmit source externally;
- do not run migrations;
- use only approved tools;
- preserve explicit user scope;
- satisfy repository validation requirements.

### Procedure

A method for performing a task:

- debugging workflow;
- review checklist;
- design evaluation process;
- test-planning method.

A skill is procedural content. It cannot promote itself into higher authority merely by containing words such as “MUST” or “override.” The host's established instruction hierarchy and authorization model remain authoritative.

Project conventions may constrain a procedure, but a repository file is not automatically trusted as policy solely because of its location. The host must determine which instructions are applicable and authorized.

## A.4 Proposed responsibility split

```text
Host/client activation coordinator
  ├── knows applicable policy
  ├── tracks selected/active local procedures
  ├── establishes delegated routing scope
  ├── validates permissions and compatibility
  └── authorizes activation or replacement
                  ↓
Hub resolver
  ├── retrieves and ranks Hub skills
  ├── accounts for declared active procedures
  ├── avoids redundant activation
  └── proposes scoped procedure activation
```

The coordinator may be implemented as a client adapter or plugin. Where only agent instructions are available, coordination is best-effort rather than mechanically enforced.

A protocol must not promise universal control over native client skills when clients do not expose activation hooks.

## A.5 One primary procedure per operation

Recommended invariant:

> One primary procedure directs a given operation at a time.

This does not prohibit distinct procedures for separate operations:

```text
diagnose failure → procedure A
implement fix   → procedure B
review fix      → procedure C
```

It also does not prohibit compatible repository constraints or explicitly scoped supporting procedures.

However, loading two independent debugging workflows for the same investigation should require an explicit composition or replacement decision.

The host should distinguish discovered, loaded, and selected-as-active skills. Reading a file is not necessarily permission to make it the primary procedure.

## A.6 Minimal activation context

Do not send the entire local catalog or all local skill content to the Hub.

Send a compact description of active procedures and, optionally, a few relevant local candidates identified by native discovery or an adapter:

```json
{
  "activation_context": {
    "active_procedures": [
      {
        "id": "local:project/payment-review",
        "role": "primary",
        "scope": "current-review",
        "capabilities": ["review-payment-changes"],
        "replaceable": false
      }
    ],
    "local_candidates": [
      {
        "id": "local:project/sql-review",
        "summary": "Review SQL changes using this repository's conventions."
      }
    ],
    "mode": "supplement-only"
  }
}
```

These are illustrative application-level identifiers, not proposed changes to official MCP skill identity semantics.

Important rules:

- `replaceable` must reflect host policy or explicit user choice, not a skill's self-declared authority.
- Missing activation context means unknown local state, not proof that no local skill exists.
- Capability summaries are claims, not guarantees of compatibility.
- Sensitive local skill content stays local by default.
- Local candidates are optional; do not force the agent to understand a full local catalog.
- If available metadata cannot establish compatibility, the Hub should request clarification or avoid supplementation rather than infer safety from names.

The simplest MVP needs only active-procedure context. Local candidate exchange can follow if benchmarks demonstrate value.

## A.7 Distinguish coverage from absence

A suitable local procedure already directing the task is not the same as an empty catalog.

Consider a separate custom resolution outcome:

```json
{
  "status": "already_covered",
  "resolution_id": "res_local_101",
  "covered_by": "local:project/payment-review",
  "reason_code": "active_procedure_covers_task"
}
```

This prevents the Hub from forcing another recommendation merely to produce a successful-looking result.

Coverage should be evaluated from the declared scope and evidence available. If coverage is uncertain, the response must not overstate that it has inspected or validated the local procedure.

This outcome is a proposed extension to the custom routing contract. It is not part of the official MCP Skills extension and is not yet a frozen product requirement.

## A.8 Explicit supplementary activation

If a useful procedure fills a distinct gap, return a scoped proposal:

```json
{
  "status": "resolved",
  "resolution_id": "res_local_102",
  "activation": {
    "mode": "supplement",
    "scope": "security-check-only",
    "preserve_primary": "local:project/payment-review"
  },
  "primary": null,
  "supporting": [
    {
      "id": "hub:payment-security",
      "applicability": "Check payment-specific security risks only."
    }
  ]
}
```

This deliberately differs from the earlier primary-first response example. If adopted, the response contract should define separate validated activation variants rather than allowing arbitrary combinations of nullable fields.

Do not silently replace the local primary or load another broad procedure and hope the model reconciles them.

Replacement should explicitly identify:

- the procedure being replaced;
- the target operation;
- the replacement procedure;
- the authority approving the change;
- whether relevant existing constraints remain in force.

Loading instructions into an LLM context is not mechanically reversible. “Deactivate” means changing the authorized active procedure and recording that decision; it does not guarantee that previously loaded text disappears from model context. A host may need context isolation or a new execution segment where strong separation is required.

## A.9 Decision policy

| Situation | Recommended behavior |
|---|---|
| User explicitly selected a local skill | Preserve it, subject to higher-priority constraints; supplement only within authorized scope |
| Active local procedure adequately covers the operation | Return `already_covered` |
| Local content provides repository conventions | Preserve applicable conventions; add a compatible Hub procedure only if needed |
| Local and Hub are identical versions of the same skill | Activate one copy |
| Same provenance, different versions | Follow pinning/update policy; do not automatically choose newest |
| Two materially contradictory procedures | Do not combine implicitly; resolve through host policy or a targeted user choice |
| Local skill is unrelated | It should not block relevant Hub routing |
| Hub procedure violates host policy | Do not activate it |
| Local activation state is unavailable | Avoid claims of complete conflict checking; use best-effort safeguards |
| Hub is unavailable | Continue under applicable local policy where possible; do not fabricate a resolution |

Explicit user selection is not a bypass for host-enforced safety or permissions.

## A.10 Identity and deduplication

Use stable identity, provenance, and version/content information where available:

- canonical skill identity;
- originating registry or repository;
- version or source revision;
- content digest;
- known fork or derivation relationship.

Do not deduplicate solely by display name.

Different content digests may indicate a customized local fork, not merely an outdated copy. Preserve that distinction before proposing replacement.

Functional overlap detection is separate from identity deduplication. Two independently authored skills may cover the same task without sharing provenance.

## A.11 Prevent routing and activation loops

Recommended safeguards:

1. Use one bootstrap instruction for Hub consultation rather than duplicating broad triggers in every skill.
2. Track a resolution identifier and current operation scope.
3. Do not resolve again solely because a loaded skill repeats the bootstrap instruction.
4. Permit re-resolution after material scope changes, capability changes, or explicit mismatch.
5. Bound clarification and replacement cycles.
6. Avoid reactivating the same skill/version for unchanged context.
7. Treat repeated local-versus-Hub oscillation as a conflict requiring abstention or explicit arbitration.

A catalog-independent consultation rule remains useful, but it should not compete with a second uncoordinated set of broad local activation instructions.

## A.12 Observability and persistence

Record, subject to privacy policy:

- what procedure was already active;
- what the Hub recommended;
- whether activation was accepted, supplemented, rejected, or replaced;
- the operation scope;
- the applicable policy revision;
- the reason for conflict or suppression.

Runtime activation state may remain disposable session data. Durable policy, approved pins, aliases, identity mappings, and curated compatibility rules should remain Git-visible and rebuildable.

Do not silently learn an enduring “local always wins” rule from temporary session events.

## A.13 MVP proposal

1. One bootstrap instruction describing when to consult the Hub.
2. Compact active-procedure context in resolution requests.
3. One primary procedure per operation.
4. No automatic replacement of a user-selected or host-locked procedure.
5. An explicit covered-by-local outcome, such as `already_covered`.
6. Supporting activation only with a clear bounded scope.
7. Loop protection tied to operation and context revision.
8. Activation decision logging for debugging.
9. Explicit documentation of enforcement limitations on clients without adapters.

Do not begin by synchronizing every local skill into the Hub. First test whether active-procedure context prevents most collisions at low cost.

## A.14 Additional prototype cases and metrics

Test:

- local and Hub copies of the same version;
- a customized local fork;
- a stale pinned local version;
- a local repository-specific review procedure versus a generic Hub review;
- contradictory sequencing instructions;
- overlapping bootstrap triggers;
- unavailable local activation metadata;
- an explicit user choice;
- a Hub recommendation that assumes unavailable capabilities;
- replacing a procedure whose instructions are already in context.

Measure:

- duplicate activation rate;
- local/Hub oscillation rate;
- unnecessary supporting-skill loads;
- preservation of user scope and local constraints;
- unresolved-conflict frequency;
- additional request tokens and latency;
- differences between adapter-backed and instruction-only clients.

## A.15 Updated architectural conclusion

The coexistence problem is not merely a conflict between two catalogs. It is a question of **who may activate a procedure for the agent**.

> **Host policy governs activation. The Hub recommends within delegated scope. The agent supplies current task context and executes the authorized procedure.**

The Hub should not claim exclusive activation authority while remaining unaware of local skills. Conversely, local storage should not grant automatic precedence over centrally curated procedures.

A small, explicit activation contract is preferable to hoping the agent will silently reconcile competing instructions.

