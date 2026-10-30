package domain

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	maxNameLength  = 100
	maxEmailLength = 254
	maxGroupLength = 50
)

var (
	// emailPattern expects an already lower-cased address.
	emailPattern = regexp.MustCompile(`^[a-z0-9._%+-]+@([a-z0-9-]+\.)+[a-z]{2,}$`)

	groupNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
)

// personalDomainPatterns match email domains of free/personal providers,
// which are rejected so only work emails can sign up. Country variants such
// as yahoo.co.in or hotmail.co.uk are covered by the optional suffix group.
// The list is not exhaustive; add patterns as needed.
var personalDomainPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^(gmail|googlemail)\.com$`),
	regexp.MustCompile(`^(yahoo|ymail|rocketmail)\.(com|[a-z]{2}|co\.[a-z]{2}|com\.[a-z]{2})$`),
	regexp.MustCompile(`^(hotmail|outlook|live|msn)\.(com|[a-z]{2}|co\.[a-z]{2}|com\.[a-z]{2})$`),
	regexp.MustCompile(`^(icloud|me|mac)\.com$`),
	regexp.MustCompile(`^(proton\.me|protonmail\.com|pm\.me)$`),
	regexp.MustCompile(`^(tutanota\.com|tuta\.io)$`),
	regexp.MustCompile(`^(zoho|zohomail|fastmail|mail|aol|inbox|gmx)\.(com|[a-z]{2})$`),
	regexp.MustCompile(`^(yandex\.[a-z]{2,3}|mail\.ru|qq\.com|163\.com|126\.com|rediffmail\.com|hey\.com|web\.de)$`),
}

// Validate trims and checks the name and email, lower-casing the email and
// rejecting personal email providers.
func (u *User) Validate() error {
	if err := u.ValidateName(); err != nil {
		return err
	}

	u.Email = strings.ToLower(strings.TrimSpace(u.Email))
	return validateWorkEmail(u.Email)
}

// SetPassword validates the plain password and stores its hash in
// PasswordHash. The plain Password is cleared so it is not kept around.
func (u *User) SetPassword(password string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	u.PasswordHash = hash
	u.Password = ""
	return nil
}

// ValidateName trims and checks the name only. Updating a user changes the
// name and never the email, which is the login identity.
func (u *User) ValidateName() error {
	u.Name = strings.TrimSpace(u.Name)

	if u.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidUser)
	}
	if utf8.RuneCountInString(u.Name) > maxNameLength {
		return fmt.Errorf("%w: name must be at most %d characters", ErrInvalidUser, maxNameLength)
	}

	return nil
}

func validateWorkEmail(email string) error {
	if email == "" {
		return fmt.Errorf("%w: email is required", ErrInvalidUser)
	}
	if len(email) > maxEmailLength || !emailPattern.MatchString(email) {
		return fmt.Errorf("%w: email is not valid", ErrInvalidUser)
	}

	_, domain, _ := strings.Cut(email, "@")
	for _, re := range personalDomainPatterns {
		if re.MatchString(domain) {
			return fmt.Errorf("%w: a work email is required", ErrInvalidUser)
		}
	}

	return nil
}

// NormalizeGroupName trims and lower-cases a group name and checks it.
func NormalizeGroupName(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))

	if name == "" {
		return "", fmt.Errorf("%w: name is required", ErrInvalidGroup)
	}
	if len(name) > maxGroupLength {
		return "", fmt.Errorf("%w: name must be at most %d characters", ErrInvalidGroup, maxGroupLength)
	}
	if !groupNamePattern.MatchString(name) {
		return "", fmt.Errorf("%w: name must start with a letter and contain only letters, digits, '_' or '-'", ErrInvalidGroup)
	}

	return name, nil
}
