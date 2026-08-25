package postgresadmin

import "testing"

func TestGeneratePasswordLengthAndUniqueness(t *testing.T) {
	pw, err := GeneratePassword()
	if err != nil {
		t.Fatal(err)
	}
	if len(pw) != 32 {
		t.Fatalf("password length = %d", len(pw))
	}
	pw2, err := GeneratePassword()
	if err != nil {
		t.Fatal(err)
	}
	if pw == pw2 {
		t.Fatal("passwords must be unique")
	}
}
