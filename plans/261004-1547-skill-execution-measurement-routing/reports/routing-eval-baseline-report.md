# Routing Evaluation Baseline Report

- **Date:** 2026-10-05
- **Commit:** `63a56879a6ab4ede8c9c5373126452fcd8d1a744`
- **Command:** `go test -v -count=1 -run RoutingEvalGate ./internal/app/`
- **Duration:** 733 ms (well under the 10 s CI target budget)

## Metrics Summary

| Metric | Measured Value | Threshold Derivation Rule | Gate Threshold (`gate-v1.json`) |
|---|---|---|---|
| **Precision@1** | `0.5976` (59.76%) | Floor to 2 decimals - 0.02 | `0.57` |
| **Recall** | `0.5833` (58.33%) | Floor to 2 decimals - 0.02 | `0.56` |
| **No-Skill Recall** | `1.0000` (100.0%) | Floor to 2 decimals - 0.02 | `0.98` |
| **No-Skill Precision** | `0.8947` (89.47%) | Reference (non-gated) | N/A |
| **False Positive Rate (FPR)** | `0.1125` (11.25%) | Ceiling to 2 decimals + 0.02 | `0.14` |

### Case Breakdown
- **Total cases:** 328
- **Positive cases (P):** 168 (42 skills × 4 examples)
- **Counter cases (N):** 126 (42 skills × 3 counter-examples)
- **No-skill cases (Z):** 34

## Failing Cases (88 total)

### Positive Misses (70 cases)
- `authentication` #3: phrase="set up secure session management with HTTP-only cookies and CSRF tokens" (status=resolved, gotPrimary=webhooks)
- `backup-recovery` #0: phrase="configure multi-region replication and snapshot retention for S3 data" (status=resolved, gotPrimary=kubernetes-operations)
- `backup-recovery` #1: phrase="design automated daily database backups with point-in-time recovery" (status=resolved, gotPrimary=kubernetes-operations)
- `browser-testing` #1: phrase="create end-to-end Playwright tests verifying the multi-step signup wizard" (status=resolved, gotPrimary=test-design)
- `cli-development` #1: phrase="build command-line tool with subcommands flags and positional arguments" (status=resolved, gotPrimary=authentication)
- `cli-development` #2: phrase="format command line output as human-readable table or structured JSON" (status=resolved, gotPrimary=logging)
- `cli-development` #3: phrase="implement bash and zsh shell tab autocompletion for CLI commands" (status=resolved, gotPrimary=authentication)
- `code-review` #0: phrase="check this patch for unintended regressions and subtle bugs" (status=resolved, gotPrimary=message-consumer-review)
- `code-review` #1: phrase="inspect the modified files in this commit for potential null pointer errors" (status=resolved, gotPrimary=message-consumer-review)
- `code-review` #2: phrase="look over this code change to verify error handling is complete" (status=no_skill, gotPrimary=)
- `code-review` #3: phrase="review the implementation of the checkout controller before merging" (status=resolved, gotPrimary=privacy-review)
- `data-analysis` #3: phrase="detect anomalies and outliers in daily transaction volume metrics" (status=resolved, gotPrimary=search-relevance)
- `dependency-upgrade` #0: phrase="migrate deprecated library calls after upgrading database driver dependency" (status=resolved, gotPrimary=database-migration)
- `dependency-upgrade` #1: phrase="update npm packages in package.json and audit for known security advisories" (status=resolved, gotPrimary=database-migration)
- `email-delivery` #0: phrase="configure DKIM SPF and DMARC DNS records to prevent email spoofing" (status=resolved, gotPrimary=authentication)
- `email-delivery` #1: phrase="handle bounce notifications and complaint feedback loops to protect domain reputation" (status=resolved, gotPrimary=authentication)
- `email-delivery` #2: phrase="integrate transactional email delivery API with template rendering" (status=resolved, gotPrimary=webhooks)
- `event-architecture-review` #1: phrase="design event schema versioning strategy for backward-compatible evolution" (status=resolved, gotPrimary=graphql-api)
- `frontend-accessibility` #1: phrase="check color contrast ratios and focus indicators across the checkout flow" (status=resolved, gotPrimary=message-consumer-review)
- `frontend-accessibility` #2: phrase="ensure assistive technologies can navigate and interact with the data table" (status=resolved, gotPrimary=privacy-review)
- `frontend-accessibility` #3: phrase="verify screen reader announcements and aria attributes on the custom dropdown" (status=resolved, gotPrimary=message-consumer-review)
- `git-workflow` #0: phrase="rebase feature branch onto latest main and resolve merge conflicts cleanly" (status=resolved, gotPrimary=kubernetes-operations)
- `git-workflow` #3: phrase="squash multiple temporary WIP commits before opening a pull request" (status=resolved, gotPrimary=kubernetes-operations)
- `image-processing` #0: phrase="apply cropping and watermark overlay to product catalog images" (status=resolved, gotPrimary=authentication)
- `image-processing` #1: phrase="batch resize and generate responsive thumbnail variants from uploaded images" (status=resolved, gotPrimary=authentication)
- `image-processing` #2: phrase="convert PNG images to WebP format with controlled lossy compression" (status=resolved, gotPrimary=authentication)
- `image-processing` #3: phrase="strip EXIF metadata and auto-orient JPEG photos uploaded by users" (status=resolved, gotPrimary=authentication)
- `incident-debugging` #0: phrase="analyze root cause of intermittent memory leaks crashing the api pod" (status=resolved, gotPrimary=performance-profiling)
- `load-testing` #2: phrase="run stress test to identify database connection pool breaking point" (status=resolved, gotPrimary=browser-testing)
- `load-testing` #3: phrase="simulate 5000 concurrent virtual users executing search queries with k6" status=resolved gotPrimary=browser-testing)
- `localization` #0: phrase="configure right-to-left layout direction and bidirectionality for Arabic" (status=resolved, gotPrimary=authentication)
- `localization` #1: phrase="extract hardcoded user-facing strings into gettext translation catalogs" (status=resolved, gotPrimary=authentication)
- `localization` #2: phrase="format currencies dates and numbers according to user locale preferences" (status=resolved, gotPrimary=webhooks)
- `localization` #3: phrase="support pluralization rules and gender agreement across European languages" (status=resolved, gotPrimary=authentication)
- `logging` #0: phrase="configure log levels and sensitive field masking to scrub customer PII" (status=resolved, gotPrimary=authentication)
- `logging` #1: phrase="inject trace and span IDs into log context for distributed request correlation" (status=resolved, gotPrimary=authentication)
- `logging` #2: phrase="set up log rotation and retention policies for local application logs" (status=resolved, gotPrimary=webhooks)
- `logging` #3: phrase="standardize structured JSON logging format across all backend services" (status=resolved, gotPrimary=cli-development)
- `machine-learning` #1: phrase="fine-tune hyperparameters and compute cross-validation F1 scores for model" (status=resolved, gotPrimary=authentication)
- `machine-learning` #2: phrase="prepare training and validation splits with feature scaling and encoding" (status=resolved, gotPrimary=authentication)
- `mcp-server` #0: phrase="define MCP tool schemas and resource templates for agent host integration" (status=resolved, gotPrimary=authentication)
- `mobile-development` #3: phrase="implement biometric authentication with FaceID and TouchID on iOS and Android" (status=resolved, gotPrimary=authentication)
- `observability` #0: phrase="add Prometheus metrics for request duration and status code counters" (status=resolved, gotPrimary=authentication)
- `observability` #1: phrase="build Grafana dashboard panels visualizing service latency percentiles" (status=resolved, gotPrimary=authentication)
- `observability` #2: phrase="configure alert thresholds on error budget burn rate and latency spikes" (status=resolved, gotPrimary=authentication)
- `observability` #3: phrase="instrument OpenTelemetry distributed tracing across HTTP service calls" (status=resolved, gotPrimary=authentication)
- `payment-integration` #0: phrase="add support for payment intent creation and idempotency keys" (status=resolved, gotPrimary=authentication)
- `payment-integration` #1: phrase="handle PayPal webhook notifications for payment capture and dispute events" (status=resolved, gotPrimary=webhooks)
- `payment-integration` #2: phrase="implement credit card processing flow with 3D Secure verification" (status=resolved, gotPrimary=webhooks)
- `performance-profiling` #0: phrase="investigate high memory allocations and garbage collection pressure in Go service" (status=resolved, gotPrimary=incident-debugging)
- `privacy-review` #0: phrase="audit personal data collection against GDPR data minimization principles" (status=resolved, gotPrimary=pull-request-review)
- `privacy-review` #1: phrase="design automated data retention schedules and user account deletion pipeline" (status=resolved, gotPrimary=pull-request-review)
- `privacy-review` #2: phrase="review consent management and third-party data tracking disclosure" (status=resolved, gotPrimary=message-consumer-review)
- `release-management` #0: phrase="coordinate multi-service release checklist and deployment artifact verification" (status=resolved, gotPrimary=kubernetes-operations)
- `security-audit` #0: phrase="audit authentication and authorization checks across all API endpoints" (status=resolved, gotPrimary=frontend-accessibility)
- `security-audit` #1: phrase="identify potential SQL injection and cross-site scripting vulnerabilities in the app" (status=resolved, gotPrimary=code-review)
- `security-audit` #2: phrase="review application permissions model against the OWASP top ten risks" (status=resolved, gotPrimary=pull-request-review)
- `security-audit` #3: phrase="scan codebase for hardcoded secrets and vulnerable third-party dependencies" (status=resolved, gotPrimary=privacy-review)
- `sql-optimization` #1: phrase="create composite index to eliminate sequential scans on large orders table" (status=resolved, gotPrimary=refactoring)
- `sql-optimization` #2: phrase="rewrite subquery into efficient CTE or window function to reduce disk I/O" (status=resolved, gotPrimary=refactoring)
- `sql-optimization` #3: phrase="tune database query parameters and resolve lock contention on row updates" (status=resolved, gotPrimary=refactoring)
- `terraform-infrastructure` #2: phrase="refactor Terraform state to split database cluster into isolated module" (status=resolved, gotPrimary=authentication)
- `terraform-infrastructure` #3: phrase="write Terraform modules to provision secure AWS S3 buckets and IAM policies" (status=resolved, gotPrimary=webhooks)
- `test-design` #0: phrase="define equivalence partitions and boundary value test cases for order validator" (status=resolved, gotPrimary=browser-testing)
- `test-design` #3: phrase="structure test cases to thoroughly exercise complex state machine transitions" (status=resolved, gotPrimary=browser-testing)
- `unit-testing` #0: phrase="add unit test suite with table-driven test cases for token verification" (status=no_skill, gotPrimary=)
- `unit-testing` #1: phrase="increase branch coverage by writing unit tests for error pathways" (status=no_skill, gotPrimary=)
- `unit-testing` #2: phrase="test boundary conditions and error returns in calculation logic" (status=resolved, gotPrimary=test-design)
- `unit-testing` #3: phrase="write comprehensive unit tests covering edge cases for the date parser" (status=no_skill, gotPrimary=)
- `video-processing` #1: phrase="generate HLS playlist segments and video thumbnails from master recording" (status=resolved, gotPrimary=authentication)

### Counter False Positives (18 cases resolved to own skill)
- `authentication` #2: phrase="review privacy policies regarding user data retention" (gotPrimary=authentication)
- `caching` #0: phrase="configure HTTP browser caching headers for static assets" (gotPrimary=caching)
- `caching` #1: phrase="design message queue retry and dead-letter handling" (gotPrimary=caching)
- `data-analysis` #2: phrase="train a random forest classifier model to predict user churn" (gotPrimary=data-analysis)
- `database-migration` #0: phrase="design the cache invalidation strategy for user session data" (gotPrimary=database-migration)
- `dependency-upgrade` #2: phrase="refactor monolithic class into separate modular services" (gotPrimary=dependency-upgrade)
- `documentation-writing` #0: phrase="generate release changelog notes from git commit tags" (gotPrimary=documentation-writing)
- `documentation-writing` #2: phrase="translate documentation into Japanese and German" (gotPrimary=documentation-writing)
- `graphql-api` #0: phrase="design REST API endpoints with OpenAPI JSON schemas" (gotPrimary=graphql-api)
- `kubernetes-operations` #0: phrase="build Docker container image for a CLI application" (gotPrimary=kubernetes-operations)
- `message-consumer-review` #2: phrase="optimize database SQL migration for consumer table" (gotPrimary=message-consumer-review)
- `privacy-review` #1: phrase="implement OAuth2 user authentication and login" (gotPrimary=privacy-review)
- `react-performance` #0: phrase="audit accessibility and keyboard navigation in React form components" (gotPrimary=react-performance)
- `react-performance` #2: phrase="write browser end-to-end tests using Playwright" (gotPrimary=react-performance)
- `refactoring` #1: phrase="upgrade third-party library to next major version" (gotPrimary=refactoring)
- `refactoring` #2: phrase="write unit tests to verify existing behavior" (gotPrimary=refactoring)
- `test-design` #2: phrase="write specific table-driven unit test assertions for date formatting helper" (gotPrimary=test-design)
- `webhooks` #0: phrase="design event-driven architecture and domain event schemas" (gotPrimary=webhooks)

### No-Skill Failures (0 cases)
All 34 no-skill cases passed with status `no_skill`.
