package domain

import (
	"errors"
	"testing"
)

func TestUserValidate(t *testing.T) {
	tests := []struct {
		name      string
		user      User
		wantEmail string
		wantErr   bool
	}{
		{"valid, email is lower-cased", User{Name: " Ann ", Email: " Ann@Corp.com "}, "ann@corp.com", false},
		{"missing name", User{Email: "ann@corp.com"}, "", true},
		{"missing email", User{Name: "Ann"}, "", true},
		{"malformed email", User{Name: "Ann", Email: "ann@corp"}, "", true},
		{"personal provider", User{Name: "Ann", Email: "ann@gmail.com"}, "", true},
		{"personal provider with country suffix", User{Name: "Ann", Email: "ann@yahoo.co.in"}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := tt.user
			err := u.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidUser) {
				t.Fatalf("expected ErrInvalidUser, got %v", err)
			}
			if err == nil && u.Email != tt.wantEmail {
				t.Fatalf("email = %q, want %q", u.Email, tt.wantEmail)
			}
		})
	}
}

func TestCheckSignInMethod(t *testing.T) {
	tests := []struct {
		name      string
		user      User
		attempted string
		wantErr   bool
		wantText  string
	}{
		{"email account, email login", User{SignupMethod: SignupMethodEmail}, SignupMethodEmail, false, ""},
		{"google account, google login", User{SignupMethod: SignupMethodGoogle}, SignupMethodGoogle, false, ""},
		{"google account, email login", User{SignupMethod: SignupMethodGoogle}, SignupMethodEmail, true, "created with google, sign in with google"},
		{"github account, email login", User{SignupMethod: SignupMethodGitHub}, SignupMethodEmail, true, "sign in with github"},
		{"email account, google login", User{SignupMethod: SignupMethodEmail}, SignupMethodGoogle, true, "sign in with your email"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckSignInMethod(tt.user, tt.attempted)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				return
			}
			if !errors.Is(err, ErrWrongSignupMethod) {
				t.Fatalf("expected ErrWrongSignupMethod, got %v", err)
			}
			if !contains(err.Error(), tt.wantText) {
				t.Fatalf("message %q should contain %q", err.Error(), tt.wantText)
			}
		})
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
