package domain

import "fmt"

// CheckSignInMethod reports whether a user may sign in with the method being
// attempted, one of the SignupMethod* constants. An account can only be used
// with the method it was created with, so an email login for a Google user
// (or the reverse) gets an error that says which method to use.
func CheckSignInMethod(u User, attempted string) error {
	if u.SignupMethod == attempted {
		return nil
	}

	use := "sign in with " + u.SignupMethod
	if u.SignupMethod == SignupMethodEmail {
		use = "sign in with your email"
	}
	return fmt.Errorf("%w: this account was created with %s, %s", ErrWrongSignupMethod, u.SignupMethod, use)
}
