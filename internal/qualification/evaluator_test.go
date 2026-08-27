package qualification

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"
	"testing"
	"time"
)

func TestTargetedRunPreservesUnaffectedEvidence(t *testing.T) {
	p := &domain.ProtocolBaseline{Devices: []domain.Device{{ID: "D", Role: "detector"}, {ID: "F", Role: "fan"}, {ID: "V", Role: "damper"}}, SequenceRules: []domain.SequenceRule{{ID: "fan", FromRole: "detector", ToRole: "fan", MaxResponseMS: 1000}, {ID: "valve", FromRole: "detector", ToRole: "damper", MaxResponseMS: 1000}}}
	now := time.Now()
	initial := domain.ExerciseRun{RunKind: "initial", Events: []domain.Event{{DeviceID: "D", At: now}, {DeviceID: "F", At: now.Add(2 * time.Second)}, {DeviceID: "V", At: now.Add(500 * time.Millisecond)}}}
	retry := domain.ExerciseRun{RunKind: "targeted", TargetRuleIDs: []string{"fan"}, Events: []domain.Event{{DeviceID: "D", At: now.Add(10 * time.Second)}, {DeviceID: "F", At: now.Add(10500 * time.Millisecond)}}}
	got := Evaluator{}.Evaluate(p, []domain.ExerciseRun{initial, retry})
	if len(got) != 2 || !got[0].Passed || !got[1].Passed {
		t.Fatalf("定向复演合并结果错误: %#v", got)
	}
}

func TestEvaluationStableOrder(t *testing.T) {
	p := &domain.ProtocolBaseline{Devices: []domain.Device{{ID: "A", Role: "a"}, {ID: "B", Role: "b"}}, SequenceRules: []domain.SequenceRule{{ID: "z", FromRole: "a", ToRole: "b", MaxResponseMS: 10}, {ID: "a", FromRole: "a", ToRole: "b", MaxResponseMS: 10}}}
	now := time.Now()
	got := Evaluator{}.Evaluate(p, []domain.ExerciseRun{{RunKind: "initial", Events: []domain.Event{{DeviceID: "A", At: now}, {DeviceID: "B", At: now.Add(time.Millisecond)}}}})
	if got[0].RuleID != "a" || got[1].RuleID != "z" {
		t.Fatalf("结果顺序不稳定: %#v", got)
	}
}

func TestFailureAttributionAndCriticalMargin(t *testing.T) {
	p := &domain.ProtocolBaseline{Devices: []domain.Device{{ID: "D", Role: "detector"}, {ID: "F", Role: "fan"}}, SequenceRules: []domain.SequenceRule{{ID: "R", FromRole: "detector", ToRole: "fan", MaxResponseMS: 3000}}}
	now := time.Now().UTC()
	inverted := Evaluator{}.Evaluate(p, []domain.ExerciseRun{{RunID: "one", RunKind: "initial", Events: []domain.Event{{DeviceID: "F", At: now}, {DeviceID: "D", At: now.Add(time.Second)}}}})
	if inverted[0].FailureCategory != "end_before_start" || len(inverted[0].CandidateWindow) != 2 {
		t.Fatalf("顺序倒置归因错误: %#v", inverted[0])
	}
	critical := Evaluator{}.Evaluate(p, []domain.ExerciseRun{{RunID: "two", RunKind: "initial", Events: []domain.Event{{DeviceID: "D", At: now}, {DeviceID: "F", At: now.Add(2900 * time.Millisecond)}}}})
	if !critical[0].Passed || !critical[0].CriticalPass || critical[0].MarginMS != 100 {
		t.Fatalf("临界裕量错误: %#v", critical[0])
	}
}
