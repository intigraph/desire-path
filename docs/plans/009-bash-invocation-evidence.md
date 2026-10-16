# Bash invocation evidence

Preserve argument-free command identities on every Bash invocation, including
successes. Reuse the existing shell AST normalizer; do not execute input or
store another copy of command arguments. Unsupported syntax and wrappers stay
UNKNOWN. An ordered program can contain commands that never execute, so these
identities describe the submitted program, not subprocess execution counts.

At ingestion, recognize bounded, line-anchored error signatures in completed
Bash output even when the enclosing tool reports success. Keep explicit native
errors authoritative. Tag output-derived failures separately: a signature is
observed output evidence, not proof of which subprocess failed or whether an
expected negative control was involved. Preserve the original response.

Weekly rates count captured invocations containing a tool, with native and
output-signature numerators separated. Disclose unknown captures and mixed
programs; never assign a chain's failure to its first command. No historical
backfill, hook registration change or production database migration is needed.
