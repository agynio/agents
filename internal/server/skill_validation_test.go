package server

import "testing"

func TestValidateSkillName(t *testing.T) {
	valid := []string{"skill", "skill-one", "s", "release-notes-2"}
	for _, name := range valid {
		if err := validateSkillName(name); err != nil {
			t.Fatalf("expected %q to be valid, got %v", name, err)
		}
	}

	invalid := []string{"", "Skill One", "skill one", "Skill", "skill_one", "-skill", "skill-", "skill/one", "."}
	for _, name := range invalid {
		if err := validateSkillName(name); err == nil {
			t.Fatalf("expected %q to be rejected", name)
		}
	}

	long := make([]byte, maxSkillNameLength+1)
	for i := range long {
		long[i] = 'a'
	}
	if err := validateSkillName(string(long)); err == nil {
		t.Fatal("expected an over-long name to be rejected")
	}
}

func TestValidateSkillDescription(t *testing.T) {
	if err := validateSkillDescription("Use when drafting release notes"); err != nil {
		t.Fatalf("expected a description to be valid, got %v", err)
	}
	for _, description := range []string{"", "   ", "\n"} {
		if err := validateSkillDescription(description); err == nil {
			t.Fatalf("expected %q to be rejected", description)
		}
	}
}
