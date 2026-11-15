package validation

import "testing"

func TestErrors(t *testing.T) {
	var errs Errors
	if errs.Any() {
		t.Error("a new list reports problems")
	}
	if got := errs.For("email"); got != "" {
		t.Errorf("message for a field with no problem = %q, want none", got)
	}

	errs.Add("password", "Enter your password")
	errs.Add("email", "Enter your email address")
	errs.Add("email", "A second problem with the email address")

	if !errs.Any() {
		t.Error("a list with problems reports none")
	}
	if got := errs.For("email"); got != "Enter your email address" {
		t.Errorf("message for email = %q, want the first one added", got)
	}
	// The error summary lists problems in the order they were added, which is the order of the page.
	if errs[0].Field != "password" || errs[1].Field != "email" {
		t.Errorf("order = %s, %s, want password then email", errs[0].Field, errs[1].Field)
	}
}
