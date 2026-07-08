package api

import (
	"net/http/httptest"
	"testing"
)

func TestStatsFiltersFromQueryParsesNewFields(t *testing.T) {
	req := httptest.NewRequest("GET", "/memory/stats?ai_provider=claude&ai_model=claude-3-5-sonnet&starred=true&shared=false&feedback_rating=thumbs_down&has_attachment=true", nil)
	filters := statsFiltersFromQuery(req)

	if filters.AIProvider != "claude" {
		t.Fatalf("expected ai_provider=claude, got %q", filters.AIProvider)
	}
	if filters.AIModel != "claude-3-5-sonnet" {
		t.Fatalf("expected ai_model=claude-3-5-sonnet, got %q", filters.AIModel)
	}
	if filters.UserStarred == nil || !*filters.UserStarred {
		t.Fatalf("expected user_starred=true, got %#v", filters.UserStarred)
	}
	if filters.UserShared == nil || *filters.UserShared {
		t.Fatalf("expected user_shared=false, got %#v", filters.UserShared)
	}
	if filters.FeedbackRating != "thumbs_down" {
		t.Fatalf("expected feedback_rating=thumbs_down, got %q", filters.FeedbackRating)
	}
	if filters.HasAttachment == nil || !*filters.HasAttachment {
		t.Fatalf("expected has_attachment=true, got %#v", filters.HasAttachment)
	}
}

func TestParseOptionalBoolSupportsCommonValues(t *testing.T) {
	truthy := []string{"true", "1", "yes", "y"}
	for _, raw := range truthy {
		value := parseOptionalBool(raw)
		if value == nil || !*value {
			t.Fatalf("expected %q to parse true", raw)
		}
	}

	falsey := []string{"false", "0", "no", "n"}
	for _, raw := range falsey {
		value := parseOptionalBool(raw)
		if value == nil || *value {
			t.Fatalf("expected %q to parse false", raw)
		}
	}

	if value := parseOptionalBool("maybe"); value != nil {
		t.Fatalf("expected maybe to parse nil, got %#v", value)
	}
}
