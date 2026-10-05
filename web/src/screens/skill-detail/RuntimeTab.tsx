import type { RuntimeHints } from '../../api/types';
import { useSkillRuntime } from '../../api/queries';
import { CommandBlock } from '../../components/CommandBlock';
import { StatusBadge } from '../../components/StatusBadge';
import { Skeleton } from '../../components/Skeleton';
import { EmptyState } from '../../components/EmptyState';

const TITLE_RUNTIME_READINESS = 'Runtime readiness';
const TITLE_CHECKLIST = 'Readiness checklist';
const TITLE_STORED_ENV = 'Stored environment variables';
const TITLE_AUTHORING_WARNINGS = 'Authoring warnings';

const LABEL_STORED_IN_ENV = 'stored in skill env';

const LABEL_READY = 'Ready';
const LABEL_SETUP_REQUIRED = 'Setup required';
const LABEL_NEEDS_REVIEW = 'Needs review';
const LABEL_UNSUPPORTED_PLATFORM = 'Unsupported platform';
const LABEL_NOT_CHECKED_YET = 'Not checked yet';
const LABEL_INACTIVE = 'Agents receive this skill only while it is active.';
const LABEL_SUPPORTED_PLATFORMS = 'Supported platforms';
const LABEL_BINARY = 'Binary: ';
const LABEL_ENV_VAR = 'Environment variable: ';
const LABEL_SETUP_CHECK = 'Setup verification check';
const LABEL_SETUP_COMMAND = 'Setup install command';
const LABEL_RECHECK_TERMINAL = 'Re-check from your terminal:';
const LABEL_DETECTED_INTERPRETERS = 'Detected interpreters: ';
const LABEL_DEPENDENCY_MANIFESTS = 'Dependency manifests: ';
const LABEL_INSTALL_CUES = 'Install cues: ';
const LABEL_ABSOLUTE_INSTALL_PATHS = 'Absolute install paths: ';
const LABEL_MISSING_LOCKFILES = 'Missing lockfiles: ';

const MSG_LOAD_FAILED = 'Failed to load runtime status.';
const MSG_NO_RUNTIME = 'This skill declares no runtime requirements.';
const MSG_MISSING_RUNTIME_WARN =
  "This skill has scripts or install instructions but no declared runtime, so agents can't preflight it";
const MSG_VALUES_NEVER_SHOWN = 'Values are never shown.';
const MSG_SETUP_COMMAND_NOTICE = 'Ask before running; installs into SKILLHUB_STATE_DIR';
const MSG_DOCTOR_NOT_RUN = 'Doctor has not run for this version.';
const MSG_ABSOLUTE_INSTALL_WARN =
  "Breaks when run from the hub's copy. Reference files relative to the skill folder ($SKILLHUB_SKILL_DIR).";
const MSG_MISSING_LOCKFILES_WARN = 'Installs are not reproducible. Commit a lockfile or pin versions.';

interface RuntimeTabProps {
  skillId: string;
  runtimeHints?: RuntimeHints;
}

export function RuntimeTab({ skillId, runtimeHints }: RuntimeTabProps) {
  const { data: status, isLoading, isError } = useSkillRuntime(skillId);

  if (isLoading) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <Skeleton height={120} />
        <Skeleton height={200} />
      </div>
    );
  }

  if (isError || !status) {
    return (
      <div className="fg-card" style={{ padding: 'var(--space-4)' }}>
        <p className="t-body" style={{ color: 'var(--color-danger)' }}>
          <span>{MSG_LOAD_FAILED}</span>
        </p>
      </div>
    );
  }

  const setupState = status.setup?.state ?? (status.served ? 'unknown' : 'not_served');
  let verdictLabel = LABEL_NOT_CHECKED_YET;
  let verdictTone: 'success' | 'warning' | 'danger' | 'neutral' = 'neutral';

  switch (setupState) {
    case 'ready':
      verdictLabel = LABEL_READY;
      verdictTone = 'success';
      break;
    case 'setup_required':
      verdictLabel = LABEL_SETUP_REQUIRED;
      verdictTone = 'warning';
      break;
    case 'review_required':
      verdictLabel = LABEL_NEEDS_REVIEW;
      verdictTone = 'warning';
      break;
    case 'unsupported_platform':
      verdictLabel = LABEL_UNSUPPORTED_PLATFORM;
      verdictTone = 'danger';
      break;
    default:
      verdictLabel = LABEL_NOT_CHECKED_YET;
      verdictTone = 'neutral';
      break;
  }

  const checkedAtTime = status.doctor?.checked_at ?? status.setup?.checked_at;
  const basisCaption = checkedAtTime
    ? `Hub checks only the platform. Binary/env results come from your terminal (basis: terminal) at ${checkedAtTime}, and may differ from your agent's shell.`
    : "Hub checks only the platform. Binary/env results come from your terminal (basis: terminal) and may differ from your agent's shell.";

  const runtimeSpec = status.runtime;
  const doctorChecks = status.doctor?.checks ?? [];
  const editCommandSuggestion = `skillhub skill edit ${skillId} --runtime-file <yaml>`;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      {/* Header Verdict Card */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        <div className="fg-card__title">
          <span>{TITLE_RUNTIME_READINESS}</span>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
          <StatusBadge label={verdictLabel} tone={verdictTone} />
          {status.setup?.reason_codes?.map((code) => (
            <StatusBadge key={code} label={code} tone="neutral" />
          ))}
        </div>

        <p className="t-caption" style={{ color: 'var(--color-text-muted)', margin: 0 }}>
          <span>{basisCaption}</span>
        </p>

        {!status.served && (
          <p className="t-body-sm" style={{ color: 'var(--color-warning)', margin: 0 }}>
            <span>{LABEL_INACTIVE}</span>
          </p>
        )}
      </section>

      {/* No Runtime Block */}
      {!runtimeSpec && (
        <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
          <EmptyState title={MSG_NO_RUNTIME} />

          {(runtimeHints?.missing_runtime_block || runtimeHints?.install_prose_detected) && (
            <div
              style={{
                backgroundColor: 'var(--color-surface-sunken)',
                padding: 'var(--space-3)',
                borderRadius: '4px',
                borderLeft: '3px solid var(--color-warning)',
              }}
            >
              <p className="t-body-sm" style={{ margin: '0 0 var(--space-2) 0', fontWeight: 'bold' }}>
                <span>{MSG_MISSING_RUNTIME_WARN}</span>
              </p>
              {runtimeHints.interpreters && runtimeHints.interpreters.length > 0 && (
                <p className="t-caption" style={{ margin: '4px 0' }}>
                  <span>{LABEL_DETECTED_INTERPRETERS}</span>
                  <span>{runtimeHints.interpreters.join(', ')}</span>
                </p>
              )}
              {runtimeHints.dependency_manifests && runtimeHints.dependency_manifests.length > 0 && (
                <p className="t-caption" style={{ margin: '4px 0' }}>
                  <span>{LABEL_DEPENDENCY_MANIFESTS}</span>
                  <span>{runtimeHints.dependency_manifests.join(', ')}</span>
                </p>
              )}
              {runtimeHints.install_cues && runtimeHints.install_cues.length > 0 && (
                <p className="t-caption" style={{ margin: '4px 0' }}>
                  <span>{LABEL_INSTALL_CUES}</span>
                  <span>{runtimeHints.install_cues.join(', ')}</span>
                </p>
              )}
              <div style={{ marginTop: 'var(--space-2)' }}>
                <code className="t-caption" style={{ fontFamily: 'var(--font-mono)' }}>
                  <span>{editCommandSuggestion}</span>
                </code>
              </div>
            </div>
          )}
        </section>
      )}

      {/* Stored env keys when no runtime spec */}
      {!runtimeSpec && status.env_keys.length > 0 && (
        <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
          <div className="fg-card__title">
            <span>{TITLE_STORED_ENV}</span>
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
            {status.env_keys.map((k) => (
              <div key={k} style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
                <code>{k}</code>
                <StatusBadge label={LABEL_STORED_IN_ENV} tone="success" />
              </div>
            ))}
          </div>
          <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
            <span>{MSG_VALUES_NEVER_SHOWN}</span>
          </span>
        </section>
      )}

      {/* Checklist Rows Card */}
      {runtimeSpec && (
        <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
          <div className="fg-card__title">
            <span>{TITLE_CHECKLIST}</span>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
            {/* Platforms row */}
            {runtimeSpec.requires?.platforms && runtimeSpec.requires.platforms.length > 0 && (
              <div
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                  padding: 'var(--space-2) 0',
                  borderBottom: '1px solid var(--color-border)',
                }}
              >
                <div>
                  <span className="t-body-sm" style={{ fontWeight: 500 }}>
                    <span>{LABEL_SUPPORTED_PLATFORMS}</span>
                  </span>
                  <div className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
                    <span>{runtimeSpec.requires.platforms.join(', ')}</span>
                  </div>
                </div>
                <StatusBadge
                  label={setupState === 'unsupported_platform' ? 'unsupported' : 'supported'}
                  tone={setupState === 'unsupported_platform' ? 'danger' : 'success'}
                />
              </div>
            )}

            {/* Bins rows */}
            {runtimeSpec.requires?.bins?.map((bin) => {
              const check = doctorChecks.find(
                (c) => c.kind === 'bin' && c.name === bin.name
              );
              let binTone: 'success' | 'danger' | 'warning' | 'neutral' = 'neutral';
              let binStatus = 'not checked';
              if (check) {
                if (check.status === 'ready') {
                  binTone = 'success';
                  binStatus = check.detail || 'ready';
                } else if (check.status === 'failed') {
                  binTone = 'danger';
                  binStatus = check.detail || 'not found';
                } else {
                  binTone = 'warning';
                  binStatus = check.detail || check.status;
                }
              }
              const constraintText = bin.version ? ` (${bin.version})` : '';

              return (
                <div
                  key={bin.name}
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    padding: 'var(--space-2) 0',
                    borderBottom: '1px solid var(--color-border)',
                  }}
                >
                  <div>
                    <span className="t-body-sm" style={{ fontWeight: 500 }}>
                      <span>{LABEL_BINARY}</span>
                      <code>{bin.name}</code>
                      <span>{constraintText}</span>
                    </span>
                    <div className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
                      <span>{binStatus}</span>
                    </div>
                  </div>
                  <StatusBadge label={check ? check.status : 'not checked'} tone={binTone} />
                </div>
              );
            })}

            {/* Env rows */}
            {runtimeSpec.requires?.env?.map((envName) => {
              const isStored = status.env_keys.includes(envName);
              const envCheck = doctorChecks.find(
                (c) => c.kind === 'env' && c.name === envName
              );
              const isTerminalPass = !isStored && envCheck?.status === 'ready';

              let envLabel = 'missing';
              let envTone: 'success' | 'neutral' | 'warning' = 'warning';
              if (isStored) {
                envLabel = 'stored in skill env';
                envTone = 'success';
              } else if (isTerminalPass) {
                envLabel = 'present in terminal env';
                envTone = 'neutral';
              }

              return (
                <div
                  key={envName}
                  style={{
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 'var(--space-2)',
                    padding: 'var(--space-2) 0',
                    borderBottom: '1px solid var(--color-border)',
                  }}
                >
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <span className="t-body-sm" style={{ fontWeight: 500 }}>
                      <span>{LABEL_ENV_VAR}</span>
                      <code>{envName}</code>
                    </span>
                    <StatusBadge label={envLabel} tone={envTone} />
                  </div>
                  {!isStored && (
                    <div style={{ marginTop: '2px' }}>
                      <CommandBlock command={status.env_set_command.replace('<KEY>', envName)} />
                    </div>
                  )}
                </div>
              );
            })}

            {runtimeSpec.requires?.env && runtimeSpec.requires.env.length > 0 && (
              <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
                <span>{MSG_VALUES_NEVER_SHOWN}</span>
              </span>
            )}

            {/* Setup check row */}
            {runtimeSpec.setup?.check && (
              <div
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                  padding: 'var(--space-2) 0',
                  borderBottom: '1px solid var(--color-border)',
                }}
              >
                <div>
                  <span className="t-body-sm" style={{ fontWeight: 500 }}>
                    <span>{LABEL_SETUP_CHECK}</span>
                  </span>
                  <div style={{ marginTop: '4px' }}>
                    <code
                      style={{
                        fontFamily: 'var(--font-mono)',
                        fontSize: '12px',
                        backgroundColor: 'var(--color-surface-sunken)',
                        padding: '2px 6px',
                        borderRadius: '4px',
                      }}
                    >
                      <span>{runtimeSpec.setup.check}</span>
                    </code>
                  </div>
                </div>
                {doctorChecks.find((c) => c.kind === 'check') && (
                  <StatusBadge
                    label={doctorChecks.find((c) => c.kind === 'check')?.status || 'checked'}
                    tone={
                      doctorChecks.find((c) => c.kind === 'check')?.status === 'ready'
                        ? 'success'
                        : 'danger'
                    }
                  />
                )}
              </div>
            )}

            {/* Setup command row */}
            {runtimeSpec.setup?.command && (
              <div
                style={{
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 'var(--space-1)',
                  padding: 'var(--space-2) 0',
                  borderBottom: '1px solid var(--color-border)',
                }}
              >
                <span className="t-body-sm" style={{ fontWeight: 500 }}>
                  <span>{LABEL_SETUP_COMMAND}</span>
                </span>
                <div style={{ marginTop: '2px' }}>
                  <code
                    style={{
                      fontFamily: 'var(--font-mono)',
                      fontSize: '12px',
                      backgroundColor: 'var(--color-surface-sunken)',
                      padding: '2px 6px',
                      borderRadius: '4px',
                    }}
                  >
                    <span>{runtimeSpec.setup.command}</span>
                  </code>
                </div>
                <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
                  <span>{MSG_SETUP_COMMAND_NOTICE}</span>
                </span>
              </div>
            )}
          </div>

          {/* Footer recheck command */}
          <div style={{ marginTop: 'var(--space-3)', display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
            <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
              <span>{LABEL_RECHECK_TERMINAL}</span>
            </span>
            <CommandBlock command={status.doctor_command} />
            {!status.doctor && (
              <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
                <span>{MSG_DOCTOR_NOT_RUN}</span>
              </span>
            )}
          </div>
        </section>
      )}

      {/* Authoring warnings */}
      {runtimeHints && (runtimeHints.absolute_install_paths?.length > 0 || runtimeHints.missing_lockfiles?.length > 0) && (
        <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
          <div className="fg-card__title">
            <span>{TITLE_AUTHORING_WARNINGS}</span>
          </div>

          {runtimeHints.absolute_install_paths && runtimeHints.absolute_install_paths.length > 0 && (
            <div
              style={{
                padding: 'var(--space-2)',
                backgroundColor: 'var(--color-surface-sunken)',
                borderLeft: '3px solid var(--color-warning)',
                borderRadius: '4px',
              }}
            >
              <span className="t-body-sm" style={{ fontWeight: 'bold' }}>
                <span>{LABEL_ABSOLUTE_INSTALL_PATHS}</span>
                <span>{runtimeHints.absolute_install_paths.join(', ')}</span>
              </span>
              <p className="t-caption" style={{ margin: '2px 0 0 0', color: 'var(--color-text-muted)' }}>
                <span>{MSG_ABSOLUTE_INSTALL_WARN}</span>
              </p>
            </div>
          )}

          {runtimeHints.missing_lockfiles && runtimeHints.missing_lockfiles.length > 0 && (
            <div
              style={{
                padding: 'var(--space-2)',
                backgroundColor: 'var(--color-surface-sunken)',
                borderLeft: '3px solid var(--color-warning)',
                borderRadius: '4px',
              }}
            >
              <span className="t-body-sm" style={{ fontWeight: 'bold' }}>
                <span>{LABEL_MISSING_LOCKFILES}</span>
                <span>{runtimeHints.missing_lockfiles.join(', ')}</span>
              </span>
              <p className="t-caption" style={{ margin: '2px 0 0 0', color: 'var(--color-text-muted)' }}>
                <span>{MSG_MISSING_LOCKFILES_WARN}</span>
              </p>
            </div>
          )}
        </section>
      )}
    </div>
  );
}
