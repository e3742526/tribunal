package tagteam

const ReviewSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["verdict", "summary", "findings", "test_suggestions"],
  "properties": {
    "verdict": {
      "type": "string",
      "enum": ["pass", "needs_changes"]
    },
    "summary": {
      "type": "string"
    },
    "findings": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["severity", "file", "issue", "fix"],
        "properties": {
          "severity": {
            "type": "string",
            "enum": ["blocker", "major", "minor", "nit"]
          },
          "file": {
            "type": "string"
          },
          "line": {
            "type": "integer"
          },
          "issue": {
            "type": "string"
          },
          "fix": {
            "type": "string"
          }
        },
        "additionalProperties": false
      }
    },
    "test_suggestions": {
      "type": "array",
      "items": {
        "type": "string"
      }
    }
  },
  "additionalProperties": false
}`
