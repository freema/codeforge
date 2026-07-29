Respond ONLY with a JSON object (no other text):
```json
{
  "verdict": "approve" | "request_changes" | "comment",
  "score": 1-10,
  "summary": "Brief overall assessment",
  "issues": [
    {"severity": "critical|major|minor|suggestion", "file": "path/to/file.go", "line": 42, "description": "What is wrong", "suggestion": "How to fix it"}
  ],
  "auto_fixable": false
}
```
