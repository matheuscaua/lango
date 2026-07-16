package domain

import "regexp"

// e164Pattern matches the same E.164 format kituo-menyu's contracts enforce
// (packages/contracts/src/validators.ts) — kept in sync deliberately, since a
// mismatch here is exactly the class of silent bug this validation exists to
// prevent (see ADR 008, "Assertividade").
var validDestinationPattern = regexp.MustCompile(`^\+[1-9]\d{1,14}(?:-\d{1,15})?(?:@g\.us)?$|^\+\d{15,20}@g\.us$`)

// IsE164 reports whether phone is a valid E.164 number (e.g. "+5511999990000") or a valid WhatsApp group JID.
func IsE164(phone string) bool {
	return validDestinationPattern.MatchString(phone)
}
