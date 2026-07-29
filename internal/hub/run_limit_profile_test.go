package hub

import "testing"

func TestApplyRunLimitProfile(t *testing.T) {
	cfg := Config{
		RunLimitProfiles: []RunLimitProfile{
			{ID: "standard", PreserveConfigured: true},
			{ID: "extended", MaxIterations: 300, TimeoutSeconds: 3600},
		},
		Supervisor: AgentConfig{MaxIterations: 25, TimeoutSeconds: 300},
		SubAgents: []AgentConfig{
			{Name: "InspectAgent", MaxIterations: 43, TimeoutSeconds: 420},
			{Name: "AnalysisAgent", MaxIterations: 15, TimeoutSeconds: 180},
		},
	}

	standard, applied, err := applyRunLimitProfile(cfg, "standard")
	if err != nil || applied {
		t.Fatalf("standard profile: applied=%v err=%v", applied, err)
	}
	if standard.SubAgents[1].MaxIterations != 15 || standard.SubAgents[1].TimeoutSeconds != 180 {
		t.Fatalf("standard profile changed configured limits: %+v", standard.SubAgents[1])
	}

	extended, applied, err := applyRunLimitProfile(cfg, "extended")
	if err != nil || !applied {
		t.Fatalf("extended profile: applied=%v err=%v", applied, err)
	}
	if extended.Supervisor.MaxIterations != 300 || extended.Supervisor.TimeoutSeconds != 3600 {
		t.Fatalf("extended supervisor limits = %+v", extended.Supervisor)
	}
	for _, agent := range extended.SubAgents {
		if agent.MaxIterations < 300 || agent.TimeoutSeconds < 3600 {
			t.Fatalf("extended sub-agent limits = %+v", agent)
		}
	}

	if cfg.Supervisor.MaxIterations != 25 || cfg.SubAgents[1].MaxIterations != 15 {
		t.Fatal("profile application mutated the source config")
	}
}

func TestApplyRunLimitProfileRejectsUnknownProfile(t *testing.T) {
	_, _, err := applyRunLimitProfile(Config{}, "missing")
	if err == nil {
		t.Fatal("expected unknown profile error")
	}
}
