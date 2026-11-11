package log

import (
	"fmt"
	"testing"
)

func TestEvent_LevelOverridesChangedWhileLogging(t *testing.T) {
	// Overrides can change while other goroutines log (log-level-overrides is hot-reloaded on
	// SIGHUP); checking an event against them must not read the overrides map unlocked
	t.Cleanup(ResetLevelOverrides)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			Tag("mytag").Field("user_id", "u_123").Loggable(DebugLevel)
		}
	}()
	for i := 0; i < 1000; i++ {
		SetLevelOverride(fmt.Sprintf("field_%d", i), "value", DebugLevel)
	}
	<-done
}
