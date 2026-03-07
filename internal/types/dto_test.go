package types

import "testing"

func TestClassificationCategoryValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  ClassificationCategory
		want ClassificationCategory
	}{
		{name: "important", got: ClassificationCategoryImportant, want: "important"},
		{name: "newsletter", got: ClassificationCategoryNewsletter, want: "newsletter"},
		{name: "junk", got: ClassificationCategoryJunk, want: "junk"},
		{name: "archive", got: ClassificationCategoryArchive, want: "archive"},
		{name: "unread_priority", got: ClassificationCategoryUnreadPriority, want: "unread_priority"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Fatalf("unexpected category value: got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestClassificationCategoryIsValid(t *testing.T) {
	t.Parallel()

	if !ClassificationCategoryImportant.IsValid() {
		t.Fatalf("ClassificationCategoryImportant.IsValid() = false, want true")
	}

	if ClassificationCategory("unknown").IsValid() {
		t.Fatalf("ClassificationCategory(\"unknown\").IsValid() = true, want false")
	}
}

func TestReviewLevelForConfidence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		confidence float64
		want       ClassificationReviewLevel
	}{
		{
			name:       "auto apply",
			confidence: 0.90,
			want:       ClassificationReviewLevelAutoApply,
		},
		{
			name:       "review",
			confidence: 0.70,
			want:       ClassificationReviewLevelReview,
		},
		{
			name:       "review with reason",
			confidence: 0.50,
			want:       ClassificationReviewLevelReviewWithReason,
		},
		{
			name:       "hold",
			confidence: 0.49,
			want:       ClassificationReviewLevelHold,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ReviewLevelForConfidence(tt.confidence); got != tt.want {
				t.Fatalf("ReviewLevelForConfidence(%0.2f) = %q, want %q", tt.confidence, got, tt.want)
			}
		})
	}
}

func TestActionKindValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  ActionKind
		want ActionKind
	}{
		{name: "label", got: ActionKindLabel, want: "label"},
		{name: "archive", got: ActionKindArchive, want: "archive"},
		{name: "delete", got: ActionKindDelete, want: "delete"},
		{name: "mark_read", got: ActionKindMarkRead, want: "mark_read"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Fatalf("unexpected action kind value: got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestClassificationSourceIsValid(t *testing.T) {
	t.Parallel()

	if !ClassificationSourceClaude.IsValid() {
		t.Fatalf("ClassificationSourceClaude.IsValid() = false, want true")
	}
	if !ClassificationSourceBlocklist.IsValid() {
		t.Fatalf("ClassificationSourceBlocklist.IsValid() = false, want true")
	}
	if ClassificationSource("unknown").IsValid() {
		t.Fatalf("ClassificationSource(\"unknown\").IsValid() = true, want false")
	}
}

func TestBlocklistKindValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  BlocklistKind
		want BlocklistKind
	}{
		{name: "sender", got: BlocklistKindSender, want: "sender"},
		{name: "domain", got: BlocklistKindDomain, want: "domain"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Fatalf("unexpected blocklist kind value: got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestBlocklistKindIsValid(t *testing.T) {
	t.Parallel()

	if !BlocklistKindSender.IsValid() {
		t.Fatalf("BlocklistKindSender.IsValid() = false, want true")
	}
	if !BlocklistKindDomain.IsValid() {
		t.Fatalf("BlocklistKindDomain.IsValid() = false, want true")
	}
	if BlocklistKind("unknown").IsValid() {
		t.Fatalf("BlocklistKind(\"unknown\").IsValid() = true, want false")
	}
}
