package app

import "testing"

func TestOptionsFromFlags(t *testing.T) {
	t.Run("rejects mixing tags and commits", func(t *testing.T) {
		_, err := OptionsFromFlags(FlagValues{
			FromTag:    "v1.0.0",
			FromCommit: "abc123",
		})
		if err == nil {
			t.Fatalf("expected error when mixing tag and commit flags")
		}
	})

	t.Run("requires a starting ref", func(t *testing.T) {
		_, err := OptionsFromFlags(FlagValues{})
		if err == nil {
			t.Fatalf("expected error when no refs provided")
		}
	})

	t.Run("defaults missing toRef to HEAD for tags", func(t *testing.T) {
		opts, err := OptionsFromFlags(FlagValues{
			FromTag: "v1.0.0",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if opts.ToTag != "HEAD" {
			t.Fatalf("expected ToTag to default to HEAD, got %q", opts.ToTag)
		}
	})

	t.Run("defaults missing toRef to HEAD for commits", func(t *testing.T) {
		opts, err := OptionsFromFlags(FlagValues{
			FromCommit: "abc123",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if opts.ToCommit != "HEAD" {
			t.Fatalf("expected ToCommit to default to HEAD, got %q", opts.ToCommit)
		}
	})

	t.Run("requires fromRef when toRef is given", func(t *testing.T) {
		_, err := OptionsFromFlags(FlagValues{ToTag: "v1.0.1"})
		if err == nil {
			t.Fatalf("expected error when toRef is provided without fromRef")
		}
	})
}
