package auth

import "testing"

func TestGeneratePasswordSatisfiesPolicy(t *testing.T) {
	for i := 0; i < 8; i++ {
		pw, err := GeneratePassword()
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidatePassword(pw, "admin"); err != nil {
			t.Fatalf("generated password rejected: %v", err)
		}
	}
}

func TestGeneratePasswordUnique(t *testing.T) {
	a, err := GeneratePassword()
	if err != nil {
		t.Fatal(err)
	}
	b, err := GeneratePassword()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two generated passwords are identical")
	}
}
