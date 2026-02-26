package review

import (
	"encoding/json"
	"regexp"
	"strings"
)

var markdownJSONBlock = regexp.MustCompile("(?s)```json\\s*\n(.*?)\n\\s*```")

// validVerdicts enumerates accepted verdict values.
var validVerdicts = map[Verdict]bool{
	VerdictApprove:        true,
	VerdictRequestChanges: true,
	VerdictComment:        true,
}

// validSeverities enumerates accepted issue severity values.
var validSeverities = map[string]bool{
	"critical":   true,
	"major":      true,
	"minor":      true,
	"suggestion": true,
}

// ParseReviewOutput extracts a ReviewResult from raw CLI output.
// It tries (in order): direct JSON, markdown code block, heuristic brace matching, and fallback.
func ParseReviewOutput(raw string) (*ReviewResult, error) {
	trimmed := strings.TrimSpace(raw)

	// Strategy 1: direct JSON unmarshal
	if r, ok := tryUnmarshal(trimmed); ok {
		return postProcess(r), nil
	}

	// Strategy 2: extract from ```json ... ``` markdown block
	if matches := markdownJSONBlock.FindStringSubmatch(raw); len(matches) > 1 {
		if r, ok := tryUnmarshal(strings.TrimSpace(matches[1])); ok {
			return postProcess(r), nil
		}
	}

	// Strategy 3: heuristic — find JSON object containing "verdict"
	if r := heuristicExtract(raw); r != nil {
		return postProcess(r), nil
	}

	// Strategy 4: fallback
	summary := trimmed
	if len(summary) > 500 {
		summary = summary[:500]
	}
	return &ReviewResult{
		Verdict: VerdictComment,
		Score:   0,
		Summary: summary,
	}, nil
}

// tryUnmarshal attempts to parse s as a ReviewResult JSON.
func tryUnmarshal(s string) (*ReviewResult, bool) {
	var r ReviewResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return nil, false
	}
	// Must have a verdict to be considered valid
	if r.Verdict == "" {
		return nil, false
	}
	return &r, true
}

// heuristicExtract finds the outermost JSON object containing "verdict" in the raw output.
func heuristicExtract(raw string) *ReviewResult {
	idx := strings.Index(raw, `"verdict"`)
	if idx < 0 {
		return nil
	}

	// Walk backwards to find opening brace
	start := -1
	for i := idx - 1; i >= 0; i-- {
		if raw[i] == '{' {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}

	// Walk forwards to find the matching closing brace
	end := -1
	depth := 0
	for i := start; i < len(raw); i++ {
		switch raw[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
		if end > 0 {
			break
		}
	}
	if end < 0 {
		return nil
	}

	r, ok := tryUnmarshal(raw[start:end])
	if !ok {
		return nil
	}
	return r
}

// postProcess normalizes a parsed ReviewResult: clamps score, validates verdict/severities.
func postProcess(r *ReviewResult) *ReviewResult {
	// Clamp score
	if r.Score < 1 {
		r.Score = 1
	}
	if r.Score > 10 {
		r.Score = 10
	}

	// Validate verdict
	if !validVerdicts[r.Verdict] {
		r.Verdict = VerdictComment
	}

	// Validate issue severities
	for i := range r.Issues {
		if !validSeverities[r.Issues[i].Severity] {
			r.Issues[i].Severity = "suggestion"
		}
	}

	return r
}
