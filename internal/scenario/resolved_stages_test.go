package scenario

import "testing"

func TestResolvedStages_Spike(t *testing.T) {
	l := Load{Type: "spike", Baseline: 10, Target: 500, Before: Duration(1), SpikeDuration: Duration(2), After: Duration(3)}
	stages, err := l.ResolvedStages()
	if err != nil {
		t.Fatal(err)
	}
	want := []Stage{
		{Duration: Duration(1), Target: 10},
		{Duration: Duration(2), Target: 500},
		{Duration: Duration(3), Target: 10},
	}
	if len(stages) != len(want) {
		t.Fatalf("expected %d stages, got %d", len(want), len(stages))
	}
	for i := range want {
		if stages[i] != want[i] {
			t.Errorf("stage %d: got %+v, want %+v", i, stages[i], want[i])
		}
	}
}

func TestResolvedStages_Stress(t *testing.T) {
	l := Load{Type: "stress", Start: 0, Step: 25, StageDuration: Duration(1), Max: 100}
	stages, err := l.ResolvedStages()
	if err != nil {
		t.Fatal(err)
	}
	wantTargets := []int{0, 25, 50, 75, 100}
	if len(stages) != len(wantTargets) {
		t.Fatalf("expected %d stages, got %d: %+v", len(wantTargets), len(stages), stages)
	}
	for i, want := range wantTargets {
		if stages[i].Target != want {
			t.Errorf("stage %d: got target %d, want %d", i, stages[i].Target, want)
		}
	}
}

func TestResolvedStages_StressAlwaysEndsAtMax(t *testing.T) {
	// Max not evenly divisible by Step: the staircase must still finish
	// exactly at Max instead of overshooting or stopping short.
	l := Load{Type: "stress", Start: 0, Step: 30, StageDuration: Duration(1), Max: 100}
	stages, err := l.ResolvedStages()
	if err != nil {
		t.Fatal(err)
	}
	last := stages[len(stages)-1]
	if last.Target != 100 {
		t.Fatalf("expected final stage target 100, got %d", last.Target)
	}
}

func TestResolvedStages_RejectsNonStageBasedType(t *testing.T) {
	l := Load{Type: "constant"}
	if _, err := l.ResolvedStages(); err == nil {
		t.Fatal("expected an error resolving stages for a non-stage-based load type")
	}
}
