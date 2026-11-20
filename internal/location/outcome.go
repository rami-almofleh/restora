package location

import "restora.local/restora/internal/snapshot"

// Outcome is what testing a location found out.
type Outcome struct {
	// Checks are the steps in order. Steps after a failure stay Pending, which means "not tried".
	Checks []snapshot.StepStatus
	// Code is empty when everything worked, otherwise one of the Code constants.
	Code    string
	Message string
	Hint    string
}

// OK reports whether every step worked.
func (o *Outcome) OK() bool {
	return o.Code == ""
}

func newOutcome(steps ...string) *Outcome {
	o := &Outcome{Checks: make([]snapshot.StepStatus, 0, len(steps))}
	for _, id := range steps {
		o.Checks = append(o.Checks, snapshot.StepStatus{ID: id, Phase: snapshot.PhasePending})
	}
	return o
}

// pass marks a step as done.
func (o *Outcome) pass(step string) {
	o.set(step, snapshot.PhaseSucceeded)
}

// fail marks a step as failed and records why. Later steps stay untried.
func (o *Outcome) fail(step, code, message, hint string) *Outcome {
	o.set(step, snapshot.PhaseFailed)
	o.Code, o.Message, o.Hint = code, message, hint
	return o
}

func (o *Outcome) set(step string, phase snapshot.Phase) {
	for i := range o.Checks {
		if o.Checks[i].ID == step {
			o.Checks[i].Phase = phase
		}
	}
}
