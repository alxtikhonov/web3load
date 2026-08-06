package scenario

import "testing"

func TestLoad_Shard_ConstantDividesVUs(t *testing.T) {
	l := Load{Type: "constant", VUs: 10, Duration: Duration(1)}
	sizes := []int{}
	for i := 0; i < 3; i++ {
		out, err := l.Shard(i, 3)
		if err != nil {
			t.Fatal(err)
		}
		sizes = append(sizes, out.VUs)
	}
	sum := sizes[0] + sizes[1] + sizes[2]
	if sum != 10 {
		t.Fatalf("expected shard VUs to sum to 10, got %d (%v)", sum, sizes)
	}
	// remainder goes to the earliest shard(s)
	if sizes[0] < sizes[1] || sizes[1] < sizes[2] {
		t.Errorf("expected non-increasing shard sizes, got %v", sizes)
	}
}

func TestLoad_Shard_ArrivalRateDividesRateAndMaxVUs(t *testing.T) {
	l := Load{Type: "arrival-rate", Rate: 100, MaxVUs: 300, Duration: Duration(1)}
	out, err := l.Shard(0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if out.Rate != 34 { // ceil-ish: 100/3=33 rem 1, shard 0 gets the remainder
		t.Errorf("expected shard 0 rate 34, got %d", out.Rate)
	}
	if out.MaxVUs != 100 {
		t.Errorf("expected shard 0 max_vus 100, got %d", out.MaxVUs)
	}
}

func TestLoad_Shard_RampingDividesStageTargets(t *testing.T) {
	l := Load{Type: "ramping", Stages: []Stage{
		{Duration: Duration(1), Target: 300},
		{Duration: Duration(2), Target: 900},
	}}
	out, err := l.Shard(1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if out.Type != "ramping" {
		t.Fatalf("expected sharded load to remain type ramping, got %q", out.Type)
	}
	if len(out.Stages) != 2 || out.Stages[0].Target != 100 || out.Stages[1].Target != 300 {
		t.Fatalf("unexpected sharded stages: %+v", out.Stages)
	}
}

func TestLoad_Shard_SpikeResolvesThenDivides(t *testing.T) {
	l := Load{Type: "spike", Baseline: 30, Target: 300, Before: Duration(1), SpikeDuration: Duration(1), After: Duration(1)}
	out, err := l.Shard(0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if out.Type != "ramping" {
		t.Fatalf("expected spike to shard down to type ramping, got %q", out.Type)
	}
	if len(out.Stages) != 3 || out.Stages[0].Target != 10 || out.Stages[1].Target != 100 || out.Stages[2].Target != 10 {
		t.Fatalf("unexpected sharded spike stages: %+v", out.Stages)
	}
}

func TestLoad_Shard_SingleShardReturnsUnchanged(t *testing.T) {
	l := Load{Type: "constant", VUs: 42, Duration: Duration(1)}
	out, err := l.Shard(0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if out.VUs != 42 {
		t.Fatalf("expected a single shard to leave VUs unchanged, got %d", out.VUs)
	}
}

func TestLoad_Shard_UnshardableTypeErrors(t *testing.T) {
	l := Load{Type: "not-a-real-type"}
	if _, err := l.Shard(0, 3); err == nil {
		t.Fatal("expected an error sharding an unrecognized load type")
	}
}
