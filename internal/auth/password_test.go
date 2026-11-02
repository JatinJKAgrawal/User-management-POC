package auth

import "testing"

func TestValidatePasswordStrength(t *testing.T) {
	cases := map[string]bool{
		"Str0ng!Pass":    true,
		"short1!A":       true,
		"Sh0rt!":         false, // too short
		"alllowercase1!": false,
		"ALLUPPER1!":     false,
		"NoDigits!!":     false,
		"NoSpecial123":   false,
	}
	for pw, ok := range cases {
		if err := ValidatePasswordStrength(pw); (err == nil) != ok {
			t.Errorf("%q: got err=%v, want ok=%v", pw, err, ok)
		}
	}
}

func TestHashAndCheck(t *testing.T) {
	h, err := HashPassword("Str0ng!Pass")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(&h, "Str0ng!Pass") {
		t.Error("correct password rejected")
	}
	if CheckPassword(&h, "wrong") {
		t.Error("wrong password accepted")
	}
	if CheckPassword(nil, "anything") {
		t.Error("nil hash accepted")
	}
}

func TestOpaqueTokenHashing(t *testing.T) {
	plain, hash, err := NewOpaqueToken()
	if err != nil {
		t.Fatal(err)
	}
	if HashOpaqueToken(plain) != hash {
		t.Error("hash mismatch")
	}
	if plain == hash {
		t.Error("plain token stored as hash")
	}
}
