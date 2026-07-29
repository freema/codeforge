You are analyzing a codebase to create or update project knowledge documentation.
{{if .UserPrompt}}
Focus area: {{.UserPrompt}}
{{end}}
## Phase 1: Analyze the repository

1. Explore the repository structure thoroughly
2. Read key files: README, configs, entry points, core modules
3. Identify: architecture patterns, tech stack, coding conventions, important abstractions
4. Focus on information that would help a new developer understand the codebase

## Phase 2: Create or update `.codeforge/` docs

Based on your analysis, create or update these files in the `.codeforge/` directory at the project root:

### `.codeforge/OVERVIEW.md`
- Project name and purpose (1-2 sentences)
- Tech stack summary
- How to run/build/test
- Key entry points

### `.codeforge/ARCHITECTURE.md`
- High-level system design
- Directory structure with descriptions
- Key abstractions and their relationships
- Data flow (request lifecycle, etc.)

### `.codeforge/CONVENTIONS.md`
- Coding patterns and style
- Error handling approach
- Testing patterns
- Naming conventions
- Configuration patterns

## Rules

- Only create or modify files inside `.codeforge/` — do NOT modify anything outside that directory
- If `.codeforge/` files already exist, UPDATE them — don't overwrite blindly, preserve accurate existing content
- Be concise — each file should be scannable, not a novel
- Focus on STABLE knowledge (architecture, patterns) not volatile details (specific line numbers)
- Use markdown with clear headers
- If the repo already has good docs (README, CONTRIBUTING), reference them rather than duplicating
