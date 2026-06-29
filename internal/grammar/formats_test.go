package grammar

import "testing"

func TestLoadFormatsEmbeddedTable(t *testing.T) {
	table := LoadFormats()
	if table.Version != "1" {
		t.Fatalf("version = %q, want 1", table.Version)
	}

	for _, kind := range []string{"event", "task", "trace", "session", "turn"} {
		spec, ok := table.Formats[kind]
		if !ok {
			t.Fatalf("missing row-kind %q", kind)
		}
		if len(spec.Fields) < 3 {
			t.Fatalf("%s fields = %d, want at least 3", kind, len(spec.Fields))
		}

		greedy := 0
		for _, field := range spec.Fields {
			if field.Width == 0 {
				greedy++
			}
			if field.Align != "left" && field.Align != "right" {
				t.Fatalf("%s.%s align = %q, want left or right", kind, field.Name, field.Align)
			}
		}
		if greedy != 1 {
			t.Fatalf("%s greedy fields = %d, want exactly 1", kind, greedy)
		}
	}
}

func TestFormatsParseAlignmentFillAndStructuredSilence(t *testing.T) {
	table := LoadFormats()

	eventSummary := fieldByName(t, table, "event", "summary")
	if eventSummary.Align != "left" || !eventSummary.Fill || eventSummary.EmptyDots {
		t.Fatalf("event.summary = %+v, want left fill=true empty_dots=false", eventSummary)
	}

	eventScore := fieldByName(t, table, "event", "score")
	if eventScore.Align != "right" || eventScore.Fill || eventScore.EmptyDots {
		t.Fatalf("event.score = %+v, want right fill=false empty_dots=false", eventScore)
	}

	taskID := fieldByName(t, table, "task", "task_id")
	if !taskID.Fill || !taskID.EmptyDots {
		t.Fatalf("task.task_id = %+v, want fill=true empty_dots=true", taskID)
	}
}

func fieldByName(t *testing.T, table FormatTable, kind, name string) FieldSpec {
	t.Helper()

	spec, ok := table.Formats[kind]
	if !ok {
		t.Fatalf("missing row-kind %q", kind)
	}
	for _, field := range spec.Fields {
		if field.Name == name {
			return field
		}
	}
	t.Fatalf("missing field %s.%s", kind, name)
	return FieldSpec{}
}
