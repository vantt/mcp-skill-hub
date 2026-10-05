import { useSkillRuntime } from '../../api/queries';
import type { RuntimeHints } from '../../api/types';
import { CommandBlock } from '../../components/CommandBlock';
import { EmptyState } from '../../components/EmptyState';
import { Skeleton } from '../../components/Skeleton';
import { StatusBadge } from '../../components/StatusBadge';

const TITLE_READINESS = 'Runtime readiness';
const VERDICT_READY = 'Ready';
const VERDICT_SETUP = 'Setup required';
const VERDICT_REVIEW = 'Needs review';
const VERDICT_UNSUPPORTED = 'Unsupported platform';
const VERDICT_UNKNOWN = 'Not checked yet';

const TEXT_UNSERVED = 'Agents receive this skill only while it is active.';
const TEXT_VALUES_NEVER_SHOWN = 'Values are never shown.';
const TEXT_NO_RUNTIME = 'This skill declares no runtime requirements.';
const TEXT_DOCTOR_NOT_RUN = 'Doctor has not run for this version.';
const TEXT_ASK_SETUP = 'Ask before running; installs into SKILLHUB_STATE_DIR';
const TEXT_RECHECK = 'Re-check from your terminal:';
const LABEL_PLATFORM_SUPPORT = 'Platform support';
const LABEL_REQUIRED_PREFIX = 'Required: ';
const LABEL_REQUIREMENT_PREFIX = 'Requirement: ';
const LABEL_HEALTH_CHECK = 'Health check';
const LABEL_SETUP_COMMAND = 'Setup command';
const TITLE_RUNTIME_SUGGESTED = 'Runtime declaration suggested';
const LABEL_DETECTED_INTERPRETERS = 'Detected interpreters: ';
const LABEL_DETECTED_MANIFESTS = 'Detected manifests: ';
const LABEL_DETECTED_CUES = 'Detected install cues: ';
const TITLE_ABSOLUTE_PATHS = 'Absolute install paths detected';
const TITLE_MISSING_LOCKFILES = 'Missing lockfiles';
const TITLE_ENV_VARS = 'Environment variables';
const LABEL_STORED_IN_SKILL_ENV = 'stored in skill env';
const MSG_MISSING_RUNTIME =
  "This skill has scripts or install instructions but no declared runtime, so agents can't preflight it";
const MSG_ABSOLUTE_PATH = 'breaks when run from the hub’s copy';
const MSG_MISSING_LOCKFILE = 'installs are not reproducible';

interface RuntimeTabProps {
  skillId: string;
  runtimeHints?: RuntimeHints;
}

export function RuntimeTab({ skillId, runtimeHints }: RuntimeTabProps) {
  const { data: status, isLoading, error } = useSkillRuntime(skillId);

  if (isLoading) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <Skeleton height="120px" />
        <Skeleton height="240px" />
      </div>
    );
  }

  if (error || !status) {
    return (
      <EmptyState
        title="Failed to load runtime status"
        description={error instanceof Error ? error.message : 'An error occurred.'}
      />
    );
  }

  const spec = status.runtime;
  const doctor = status.doctor;
  const checks = doctor?.checks || [];
  const editCommand = `skillhub skill edit ${skillId} --runtime-file <yaml>`;
  const getCheck = (kind: string, name?: string) => {
    return checks.find((c) => c.kind === kind && (!name || c.name === name));
  };

  // Header verdict mapping
  let verdictLabel = VERDICT_UNKNOWN;
  let verdictTone: 'success' | 'warning' | 'danger' | 'neutral' = 'neutral';

  const setupState = status.setup?.state || 'unknown';
  switch (setupState) {
  case 'ready':
    verdictLabel = VERDICT_READY;
    verdictTone = 'success';
    break;
  case 'setup_required':
    verdictLabel = VERDICT_SETUP;
    verdictTone = 'warning';
    break;
  case 'review_required':
    verdictLabel = VERDICT_REVIEW;
    verdictTone = 'warning';
    break;
  case 'unsupported_platform':
    verdictLabel = VERDICT_UNSUPPORTED;
    verdictTone = 'danger';
    break;
  default:
    verdictLabel = VERDICT_UNKNOWN;
    verdictTone = 'neutral';
    break;
  }

  const checkedAtTime = doctor?.checked_at
    ? ` at ${doctor.checked_at}`
    : '';

  const subtitleText = `Hub checks only the platform. Binary/env results come from your terminal (basis: terminal)${checkedAtTime}, and may differ from your agent's shell.`;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      {/* Header Verdict Card */}
      <section
        className="fg-card"
        style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}
      >
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: 'var(--space-2)',
          }}
        >
          <div className="fg-card__title">
            <span>{TITLE_READINESS}</span>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
            <StatusBadge label={verdictLabel} tone={verdictTone} />
            {status.setup?.reason_codes &&
              status.setup.reason_codes.map((rc) => (
                <span
                  key={rc}
                  className="t-caption"
                  style={{ color: 'var(--color-text-subtle)' }}
                >
                  <span>{rc}</span>
                </span>
              ))}
          </div>
        </div>

        <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
          <span>{subtitleText}</span>
        </span>

        {!status.served && (
          <span className="t-caption" style={{ color: 'var(--color-warning)' }}>
            <span>{TEXT_UNSERVED}</span>
          </span>
        )}
      </section>

      {/* Checklist Rows if runtime block declared */}
      {spec ? (
        <section
          className="fg-card"
          style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}
        >
          {/* Platforms */}
          {spec.requires?.platforms && spec.requires.platforms.length > 0 && (
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                padding: '8px 0',
                borderBottom: '1px solid var(--color-border)',
              }}
            >
              <div style={{ display: 'flex', flexDirection: 'column', gap: '2px' }}>
                <span className="t-body-sm" style={{ fontWeight: 600 }}>
                  <span>{LABEL_PLATFORM_SUPPORT}</span>
                </span>
                <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
                  <span>{LABEL_REQUIRED_PREFIX}</span>
                  <span>{spec.requires.platforms.join(', ')}</span>
                </span>
              </div>
              <div>
                {(() => {
                  const check = getCheck('platform');
                  if (check) {
                    return (
                      <StatusBadge
                        label={check.status === 'pass' ? 'pass' : 'unsupported'}
                        tone={check.status === 'pass' ? 'success' : 'danger'}
                      />
                    );
                  }
                  return <StatusBadge label="not checked" tone="neutral" />;
                })()}
              </div>
            </div>
          )}

          {/* Binaries */}
          {spec.requires?.bins &&
            spec.requires.bins.map((bin) => {
              const check = getCheck('bin', bin.name);
              let evidence = 'not checked';
              let tone: 'success' | 'danger' | 'neutral' = 'neutral';
              if (check) {
                evidence = check.detail || check.status;
                tone = check.status === 'pass' ? 'success' : 'danger';
              }
              const reqText = bin.version
                ? `${bin.name} (${bin.version})`
                : bin.name;
              return (
                <div
                  key={bin.name}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    padding: '8px 0',
                    borderBottom: '1px solid var(--color-border)',
                  }}
                >
                  <div style={{ display: 'flex', flexDirection: 'column', gap: '2px' }}>
                    <span className="t-body-sm" style={{ fontWeight: 600 }}>
                      <code>{bin.name}</code>
                    </span>
                    <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
                      <span>{LABEL_REQUIREMENT_PREFIX}</span>
                      <span>{reqText}</span>
                    </span>
                  </div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
                    <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
                      <span>{evidence}</span>
                    </span>
                    <StatusBadge label={check ? check.status : 'not checked'} tone={tone} />
                  </div>
                </div>
              );
            })}

          {/* Environment Variables */}
          {spec.requires?.env && spec.requires.env.length > 0 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
              {spec.requires.env.map((envKey) => {
                const check = getCheck('env', envKey);
                const isStored = status.env_keys.includes(envKey);
                let envLabel = 'missing';
                let envTone: 'success' | 'info' | 'danger' = 'danger';

                if (isStored) {
                  envLabel = 'stored in skill env';
                  envTone = 'success';
                } else if (check?.status === 'pass') {
                  envLabel = 'present in terminal env';
                  envTone = 'info';
                }

                const cmd = `skillhub skill env set ${skillId} ${envKey}`;
                return (
                  <div
                    key={envKey}
                    style={{
                      display: 'flex',
                      flexDirection: 'column',
                      gap: 'var(--space-1)',
                      padding: '8px 0',
                      borderBottom: '1px solid var(--color-border)',
                    }}
                  >
                    <div
                      style={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                      }}
                    >
                      <span className="t-body-sm" style={{ fontWeight: 600 }}>
                        <code>{envKey}</code>
                      </span>
                      <StatusBadge label={envLabel} tone={envTone} />
                    </div>
                    {envLabel === 'missing' && <CommandBlock command={cmd} />}
                  </div>
                );
              })}
              <span className="t-caption" style={{ color: 'var(--color-text-subtle)', marginTop: '2px' }}>
                <span>{TEXT_VALUES_NEVER_SHOWN}</span>
              </span>
            </div>
          )}

          {/* Setup Check & Command */}
          {spec.setup && (spec.setup.check || spec.setup.command) && (
            <div
              style={{
                display: 'flex',
                flexDirection: 'column',
                gap: 'var(--space-2)',
                padding: '8px 0',
                borderBottom: '1px solid var(--color-border)',
              }}
            >
              {spec.setup.check && (
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: '2px' }}>
                    <span className="t-body-sm" style={{ fontWeight: 600 }}>
                    <span>{LABEL_HEALTH_CHECK}</span>
                    </span>
                    <span className="t-caption">
                      <code>{spec.setup.check}</code>
                    </span>
                  </div>
                  <div>
                    {(() => {
                      const check = getCheck('check');
                      if (check) {
                        return (
                          <StatusBadge
                            label={check.status === 'pass' ? 'pass' : 'failed'}
                            tone={check.status === 'pass' ? 'success' : 'danger'}
                          />
                        );
                      }
                      return <StatusBadge label="not checked" tone="neutral" />;
                    })()}
                  </div>
                </div>
              )}

              {spec.setup.command && (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-1)' }}>
                  <span className="t-body-sm" style={{ fontWeight: 600 }}>
                  <span>{LABEL_SETUP_COMMAND}</span>
                  </span>
                  <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
                    <span>{TEXT_ASK_SETUP}</span>
                  </span>
                  <code style={{ fontSize: '12px', padding: '4px 6px', background: 'var(--color-surface-sunken)', borderRadius: '4px' }}>
                    {spec.setup.command}
                  </code>
                </div>
              )}
            </div>
          )}

          {/* Footer: Doctor command */}
          <div
            style={{
              display: 'flex',
              flexDirection: 'column',
              gap: 'var(--space-2)',
              paddingTop: 'var(--space-2)',
            }}
          >
            <span className="t-body-sm" style={{ color: 'var(--color-text-muted)' }}>
              <span>{TEXT_RECHECK}</span>
            </span>
            <CommandBlock command={status.doctor_command} />
            {!doctor && (
              <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
                <span>{TEXT_DOCTOR_NOT_RUN}</span>
              </span>
            )}
          </div>
        </section>
      ) : (
        <>
          {/* Empty State: No runtime block declared */}
          <EmptyState title={TEXT_NO_RUNTIME} />

          {/* Stored Environment Variables if any */}
          {status.env_keys && status.env_keys.length > 0 && (
            <section
              className="fg-card"
              style={{
                display: 'flex',
                flexDirection: 'column',
                gap: 'var(--space-3)',
              }}
            >
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  gap: 'var(--space-2)',
                }}
              >
                <div className="fg-card__title">
                  <span>{TITLE_ENV_VARS}</span>
                </div>
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
                {status.env_keys.map((envKey) => (
                  <div
                    key={envKey}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      padding: '8px 0',
                      borderBottom: '1px solid var(--color-border)',
                    }}
                  >
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '2px' }}>
                      <span className="t-body-sm" style={{ fontWeight: 600 }}>
                        <code>{envKey}</code>
                      </span>
                    </div>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
                      <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
                        <span>{LABEL_STORED_IN_SKILL_ENV}</span>
                      </span>
                      <StatusBadge label={LABEL_STORED_IN_SKILL_ENV} tone="success" />
                    </div>
                  </div>
                ))}
              </div>
              <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
                <span>{TEXT_VALUES_NEVER_SHOWN}</span>
              </span>
            </section>
          )}
        </>
      )}

      {/* Authoring & Missing Runtime Warnings */}
      {(!spec && (runtimeHints?.missing_runtime_block || runtimeHints?.install_prose_detected)) && (
        <section
          className="fg-card"
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 'var(--space-2)',
            borderLeft: '4px solid var(--color-warning)',
          }}
        >
          <div className="fg-card__title" style={{ color: 'var(--color-warning)' }}>
            <span>{TITLE_RUNTIME_SUGGESTED}</span>
          </div>
          <span className="t-body-sm">
            <span>{MSG_MISSING_RUNTIME}</span>
          </span>
          <div className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
            {runtimeHints.interpreters && runtimeHints.interpreters.length > 0 && (
              <div>
                <span>{LABEL_DETECTED_INTERPRETERS}</span>
                <span>{runtimeHints.interpreters.join(', ')}</span>
              </div>
            )}
            {runtimeHints.dependency_manifests && runtimeHints.dependency_manifests.length > 0 && (
              <div>
                <span>{LABEL_DETECTED_MANIFESTS}</span>
                <span>{runtimeHints.dependency_manifests.join(', ')}</span>
              </div>
            )}
            {runtimeHints.install_cues && runtimeHints.install_cues.length > 0 && (
              <div>
                <span>{LABEL_DETECTED_CUES}</span>
                <span>{runtimeHints.install_cues.join(', ')}</span>
              </div>
            )}
          </div>
          <div style={{ marginTop: 'var(--space-1)' }}>
            <code style={{ fontSize: '12px' }}>{editCommand}</code>
          </div>
        </section>
      )}

      {/* Absolute Install Paths Warning */}
      {runtimeHints?.absolute_install_paths && runtimeHints.absolute_install_paths.length > 0 && (
        <section
          className="fg-card"
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 'var(--space-2)',
            borderLeft: '4px solid var(--color-warning)',
          }}
        >
          <div className="fg-card__title" style={{ color: 'var(--color-warning)' }}>
            <span>{TITLE_ABSOLUTE_PATHS}</span>
          </div>
          <span className="t-body-sm">
            <span>{runtimeHints.absolute_install_paths.join(', ')}</span>
            <span> — </span>
            <span>{MSG_ABSOLUTE_PATH}</span>
          </span>
        </section>
      )}

      {/* Missing Lockfiles Warning */}
      {runtimeHints?.missing_lockfiles && runtimeHints.missing_lockfiles.length > 0 && (
        <section
          className="fg-card"
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 'var(--space-2)',
            borderLeft: '4px solid var(--color-warning)',
          }}
        >
          <div className="fg-card__title" style={{ color: 'var(--color-warning)' }}>
            <span>{TITLE_MISSING_LOCKFILES}</span>
          </div>
          <span className="t-body-sm">
            <span>{runtimeHints.missing_lockfiles.join(', ')}</span>
            <span> — </span>
            <span>{MSG_MISSING_LOCKFILE}</span>
          </span>
        </section>
      )}
    </div>
  );
}
