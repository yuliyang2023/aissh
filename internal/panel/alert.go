package panel

import (
	"errors"
	"math"
	"regexp"
)

// AlertCondition belongs to a single monitor in a single live session. It is
// evaluated on fresh collector output, never on AI-authored display content.
type AlertCondition struct {
	Enabled           bool    `json:"enabled"`
	Kind              string  `json:"kind"` // match, not_match, above, below
	Pattern           string  `json:"pattern"`
	Threshold         float64 `json:"threshold"`
	RecoveryThreshold float64 `json:"recoveryThreshold"`
	Severity          string  `json:"severity"`
	ForSeconds        int     `json:"forSeconds"`
	RecoverySeconds   int     `json:"recoverySeconds"`
}

func (c AlertCondition) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.Severity != "warning" && c.Severity != "critical" {
		return errors.New("提醒级别须为 warning 或 critical")
	}
	if c.ForSeconds < 0 || c.ForSeconds > 3600 || c.RecoverySeconds < 0 || c.RecoverySeconds > 3600 {
		return errors.New("异常和恢复持续时间须为 0–3600 秒")
	}
	if c.Pattern == "" || len(c.Pattern) > 256 {
		return errors.New("提醒匹配表达式须为 1–256 字节")
	}
	re, err := regexp.Compile(c.Pattern)
	if err != nil {
		return errors.New("提醒匹配表达式无效")
	}
	switch c.Kind {
	case "match", "not_match":
	case "above", "below":
		if re.NumSubexp() != 1 {
			return errors.New("数值提醒必须用一个捕获组提取数值")
		}
		for _, v := range []float64{c.Threshold, c.RecoveryThreshold} {
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e12 {
				return errors.New("提醒阈值无效")
			}
		}
		if (c.Kind == "above" && c.RecoveryThreshold > c.Threshold) || (c.Kind == "below" && c.RecoveryThreshold < c.Threshold) {
			return errors.New("恢复阈值应位于异常阈值的正常一侧")
		}
	default:
		return errors.New("不支持的提醒条件，可使用 match、not_match、above 或 below")
	}
	return nil
}
