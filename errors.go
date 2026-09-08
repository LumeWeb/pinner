package pinner

import "errors"

// ErrConfirmRequired is returned by Invoke when a SafetyDestructive operation
// is invoked without the confirmation flag set (AgentConfirm opted-in args
// excepted). Callers gate on it with errors.Is to drive confirm prompts.
var ErrConfirmRequired = errors.New("confirmation required")

// ErrHumanRequired is returned by Invoke when an InteractionHumanOnly
// operation is invoked without a human prompter available. Callers gate on it
// with errors.Is to drive hand-off flows.
var ErrHumanRequired = errors.New("operation requires a human")

// ErrSelector is returned when a SelectionGroup receives more or fewer than
// exactly one selected member.
var ErrSelector = errors.New("selector group must select exactly one member")
