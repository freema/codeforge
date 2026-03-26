package prompt

// AnalyzeRepoPrompt is the prompt template for analyzing a repository's structure.
// Exported for reuse by the CI Action's knowledge_update task type.
const AnalyzeRepoPrompt = `You are analyzing a codebase to understand its architecture, conventions, and key components.

{{if .Params.focus}}Focus area: {{.Params.focus}}{{end}}

## Instructions

1. Explore the repository structure thoroughly
2. Read key files: README, configs, entry points, core modules
3. Identify: architecture patterns, tech stack, coding conventions, important abstractions
4. Summarize your findings — this will be used to generate documentation

## Rules

- Do NOT modify any files
- Be thorough but concise in your analysis
- Focus on information that would help a new developer understand the codebase`

// UpdateKnowledgePrompt is the prompt template for creating/updating .codeforge/ knowledge docs.
// Exported for reuse by the CI Action's knowledge_update task type.
const UpdateKnowledgePrompt = `You are a technical writer creating project knowledge documentation.

Based on your analysis of this codebase, create or update documentation files in the ` + "`.codeforge/`" + ` directory.

{{if .Params.focus}}Focus area: {{.Params.focus}}{{end}}

## Files to create/update

Create these files in ` + "`.codeforge/`" + ` directory at the project root:

### ` + "`.codeforge/OVERVIEW.md`" + `
- Project name and purpose (1-2 sentences)
- Tech stack summary
- How to run/build/test
- Key entry points

### ` + "`.codeforge/ARCHITECTURE.md`" + `
- High-level system design
- Directory structure with descriptions
- Key abstractions and their relationships
- Data flow (request lifecycle, etc.)

### ` + "`.codeforge/CONVENTIONS.md`" + `
- Coding patterns and style
- Error handling approach
- Testing patterns
- Naming conventions
- Configuration patterns

## Rules

- If ` + "`.codeforge/`" + ` files already exist, UPDATE them — don't overwrite blindly, preserve accurate existing content
- Be concise — each file should be scannable, not a novel
- Focus on STABLE knowledge (architecture, patterns) not volatile details (specific line numbers)
- Use markdown with clear headers
- If the repo already has good docs (README, CONTRIBUTING), reference them rather than duplicating`
