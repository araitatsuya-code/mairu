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
