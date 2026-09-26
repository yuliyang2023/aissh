package panel

import "testing"

func TestAlertConditionValidation(t *testing.T) {
	valid := AlertCondition{Enabled: true, Kind: "above", Pattern: `VALUE=([0-9.]+)`, Threshold: 90, RecoveryThreshold: 85, Severity: "warning", ForSeconds: 10, RecoverySeconds: 10}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*AlertCondition){
		func(c *AlertCondition) { c.Pattern = "[" },
		func(c *AlertCondition) { c.Pattern = "no-capture" },
		func(c *AlertCondition) { c.Pattern = "(one)(two)" },
		func(c *AlertCondition) { c.RecoveryThreshold = 95 },
		func(c *AlertCondition) { c.ForSeconds = -1 },
		func(c *AlertCondition) { c.Severity = "unknown" },
	} {
		c := valid
		change(&c)
		if c.Validate() == nil {
			t.Fatalf("invalid condition accepted: %#v", c)
		}
	}
	if err := (AlertCondition{Enabled: false}).Validate(); err != nil {
		t.Fatal(err)
	}
}
