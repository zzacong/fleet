---
name: choose-flow
description: Estimate the size of the current work and recommend whether to use /implement, /to-spec, or /to-tickets next.
disable-model-invocation: true
---

# Choose Flow

Estimate the work from the conversation and, when useful, a quick look at the
codebase. Recommend the next step, not the entire eventual path:

- `/implement` for clear, coherent work that fits one session.
- `/to-spec` for work spanning multiple sessions or needing shared decisions.
- `/to-tickets` only when a stable spec or plan exists and the work needs two or
  more independently implementable tickets. Otherwise choose `/to-spec` first.

Return:

1. **Recommendation:** the command to run next
2. **Estimate:** scope, focused sessions, and confidence
3. **Why:** the strongest signals and what would change the recommendation

Be concise. State assumptions when uncertain; do not start the recommended flow
or interview the user.
