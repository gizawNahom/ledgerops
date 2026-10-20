package support

// SCAFFOLD: true
//
// Scaffold is the RED-scaffold marker for non-HTTP production stubs created by
// DISTILL (Mandate 7). It panics with a message the acceptance suite
// recognises, so a scenario that reaches an unimplemented operation fails as
// MISSING_FUNCTIONALITY rather than as a setup error. DELIVER deletes each
// call as it implements the operation; zero "SCAFFOLD: true" markers must
// remain when the feature is done.
func Scaffold(operation string) {
	panic("__SCAFFOLD__: " + operation + " not yet implemented -- RED scaffold")
}
