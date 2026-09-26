package widgets

import (
	"encoding/json"
	"git.example.com/ai/wrd-ai-cli/internal/monitorplan"
	"time"
)

func RecordDataset(previous Data, dataset monitorplan.Dataset, definition monitorplan.Definition, now time.Time) Data {
	next := previous
	next.Sequence++
	next.Data = dataset.Bind(definition.Bindings)
	next.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	next.Error = ""
	next.Issues = []string{}
	for _, d := range dataset.Diagnostics {
		next.Issues = append(next.Issues, d.Stage+" / "+d.NodeID+": "+d.Message)
	}
	good := true
	for _, m := range dataset.Metrics {
		good = good && m.Quality == "ok"
	}
	for _, t := range dataset.Tables {
		good = good && t.Quality == "ok"
	}
	if good {
		next.LastGood = next.Data
		next.LastGoodAt = next.UpdatedAt
	} else {
		next.LastGood = staleFields(previous.LastGood)
	}
	outputs := map[string]monitorplan.Output{}
	for _, o := range definition.DataPlan.Outputs {
		outputs[o.ID] = o
	}
	values := map[string]*float64{}
	for _, b := range definition.Bindings {
		if outputs[b.OutputID].History {
			var ptr *float64
			m := dataset.Metrics[b.OutputID]
			if n, ok := m.Value.(float64); ok && m.Quality == "ok" {
				v := n
				ptr = &v
			}
			values[b.Prop] = ptr
		}
	}
	history := append([]Point(nil), previous.History...)
	if len(values) > 0 {
		if len(history) > 0 {
			last, _ := time.Parse(time.RFC3339Nano, history[len(history)-1].Time)
			gap := now.Sub(last)
			if gap < 0 || gap > time.Duration(max(15000, next.IntervalMS*3))*time.Millisecond {
				history = append(history, Point{Time: last.Add(time.Millisecond).UTC().Format(time.RFC3339Nano), Values: map[string]*float64{}})
			}
		}
		history = append(history, Point{Time: next.UpdatedAt, Values: values})
	}
	for len(history) > 0 {
		t, _ := time.Parse(time.RFC3339Nano, history[0].Time)
		b, _ := json.Marshal(history)
		if len(history) <= 600 && len(b) <= MaxHistory && now.Sub(t) <= 10*time.Minute {
			break
		}
		history = history[1:]
		next.Truncated = true
	}
	next.History = history
	return next
}

func staleFields(fields map[string]any) map[string]any {
	if fields == nil {
		return nil
	}
	raw, _ := json.Marshal(fields)
	var result map[string]any
	_ = json.Unmarshal(raw, &result)
	for _, v := range result {
		if field, ok := v.(map[string]any); ok {
			field["quality"] = "stale"
		}
	}
	return result
}
func Stale(previous Data, reason string) Data {
	next := previous
	next.Sequence++
	next.Error = reason
	next.Data = staleFields(previous.Data)
	next.LastGood = staleFields(previous.LastGood)
	return next
}
