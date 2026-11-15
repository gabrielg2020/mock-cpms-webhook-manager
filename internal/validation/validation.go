package validation

type FieldError struct {
	Field   string
	Message string
}

type Errors []FieldError

func (errs *Errors) Add(field, message string) {
	*errs = append(*errs, FieldError{Field: field, Message: message})
}

// For returns the first message for a field, or "" if it has none.
func (errs Errors) For(field string) string {
	for _, fieldError := range errs {
		if fieldError.Field == field {
			return fieldError.Message
		}
	}
	return ""
}

func (errs Errors) Any() bool {
	return len(errs) > 0
}
