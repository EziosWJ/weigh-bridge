package weighing

// IsBusinessPlate reports whether value is an accepted local vehicle plate
// after removing separators and normalizing case. The simulator uses the same
// rule as task creation and manual plate confirmation.
func IsBusinessPlate(value string) bool {
	return businessPlate.MatchString(normalizePlate(value))
}
