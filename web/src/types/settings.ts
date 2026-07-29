// Runtime server settings (Settings → operator-only endpoints).

// GET/PUT /settings/review — defaults for webhook-triggered PR reviews.
// default_* are the stored overrides (empty = not set); effective_* are the
// values that actually apply after the fallback chain
// (runtime settings → code_review.default_cli config → built-in default).
export interface ReviewSettings {
  default_cli: string;
  default_model: string;
  effective_cli: string;
  effective_model: string;
}

export interface UpdateReviewSettingsRequest {
  default_cli: string;
  default_model: string;
}
