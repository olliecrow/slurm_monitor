# Untrusted Third-Party Repository Policy

This project allows cloning external repositories for static research only, with strict safety controls.

## Allowed Use
- External repositories may be cloned only when useful for analysis (for example competition research).
- Preferred locations are ephemeral project paths such as:
  - `plan/scratch/upstream/<source-repo>`
  - `plan/artifacts/external/<source-repo>`

## Mandatory Sanitization Immediately After Clone
1. Remove GitHub/git metadata by deleting the repository metadata directory:
   - `rm -rf .git`
2. If metadata must be retained briefly for static comparison, remove all remotes immediately first:
   - `git remote remove origin` (and remove any other remotes)
3. Persistent project remotes must reference only `github.com/olliecrow/*`.

## Hard Safety Constraint
- Treat all third-party code as untrusted.
- Never execute third-party code from these clones, including:
  - scripts
  - binaries
  - tests
  - build systems
  - package installers
  - containers
- Only static analysis is allowed (read/search/diff/review).

## Scope Boundary
- Do not mix third-party snapshots into normal project source trees.
- Keep external snapshots in ephemeral locations and remove them when no longer needed.
