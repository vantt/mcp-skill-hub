package cli

// workspaceEnvVar names the environment variable that supplies a default
// workspace when --workspace is omitted. The flag wins over the variable, and
// the variable wins over upward discovery from the current directory. Home
// directories are never scanned.
const workspaceEnvVar = "SKILLHUB_WORKSPACE"
