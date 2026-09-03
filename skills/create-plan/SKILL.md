---
name: create-plan
description: Create a numbered implementation plan in `.plans/` when the user asks to plan, outline, or sequence work.
disable-model-invocation: true
---

# Create Plan

When creating a plan document:

1. Inspect `.plans/` and identify the highest existing numeric prefix.
2. Assign the next available sequential number, formatted as exactly two digits, e.g. `01`, `02`, `03`.
3. Use a short, descriptive kebab-case name for the plan.
4. Name the file using this format: `.plans/NN-short-kebab-case-description.md`  
   For example: `.plans/08-add-user-authentication.md`
5. Create the `.plans/` directory if it does not already exist.
6. Keep plan files in `.plans/`; do not create them in the project root or another directory.
7. Do not overwrite an existing plan file. If the expected filename already exists, choose the next unused number.

The plan is complete when it exists at the next unused numbered path under `.plans/` and contains the requested plan.
