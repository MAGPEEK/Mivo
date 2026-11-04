package scantrigger

// SetAfterTick makes Run call afterTick when it has handled a tick, so tests know that the
// effects of a tick are complete before they assert.
func SetAfterTick(trigger *Trigger, afterTick func()) {
	trigger.afterTick = afterTick
}
