// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ocagent_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/tools/internal/telemetry/event"
	"golang.org/x/tools/internal/telemetry/export"
)

func TestTrace(t *testing.T) {
	start, _ := time.Parse(time.RFC3339Nano, "1970-01-01T00:00:30Z")
	at, _ := time.Parse(time.RFC3339Nano, "1970-01-01T00:00:40Z")
	end, _ := time.Parse(time.RFC3339Nano, "1970-01-01T00:00:50Z")
	const prefix = testNodeStr + `
		"spans":[{
			"trace_id":"AAAAAAAAAAAAAAAAAAAAAA==",
			"span_id":"AAAAAAAAAAA=",
			"parent_span_id":"AAAAAAAAAAA=",
			"name":{"value":"event span"},
			"start_time":"1970-01-01T00:00:30Z",
			"end_time":"1970-01-01T00:00:50Z",
			"time_events":{
	`
	const suffix = `
			},
			"same_process_as_parent_span":true
		}]
	}`
	tests := []struct {
		name  string
		event func(ctx context.Context) event.Event
		want  string
	}{
		{
			name: "no tags",
			event: func(ctx context.Context) event.Event {
				return event.Event{
					At: at,
				}
			},
			want: prefix + `
						"timeEvent":[{"time":"1970-01-01T00:00:40Z"}]
			` + suffix,
		},
		{
			name: "description no error",
			event: func(ctx context.Context) event.Event {
				return event.Event{
					At:      at,
					Message: "cache miss",
					Tags: event.TagList{
						event.TagOf("db", "godb"),
					},
				}
			},
			want: prefix + `"timeEvent":[{"time":"1970-01-01T00:00:40Z","annotation":{
  "description": { "value": "cache miss" },
  "attributes": {
    "attributeMap": {
      "db": { "stringValue": { "value": "godb" } }
    }
  }
}}]` + suffix,
		},

		{
			name: "description and error",
			event: func(ctx context.Context) event.Event {
				return event.Event{
					At:      at,
					Message: "cache miss",
					Error:   errors.New("no network connectivity"),
					Tags: event.TagList{
						event.TagOf("db", "godb"), // must come before e
					},
				}
			},
			want: prefix + `"timeEvent":[{"time":"1970-01-01T00:00:40Z","annotation":{
  "description": { "value": "cache miss" },
  "attributes": {
    "attributeMap": {
      "db": { "stringValue": { "value": "godb" } },
      "error": { "stringValue": { "value": "no network connectivity" } }
    }
  }
	}}]` + suffix,
		},
		{
			name: "no description, but error",
			event: func(ctx context.Context) event.Event {
				return event.Event{
					At:    at,
					Error: errors.New("no network connectivity"),
					Tags: event.TagList{
						event.TagOf("db", "godb"),
					},
				}
			},
			want: prefix + `"timeEvent":[{"time":"1970-01-01T00:00:40Z","annotation":{
  "description": { "value": "no network connectivity" },
  "attributes": {
    "attributeMap": {
      "db": { "stringValue": { "value": "godb" } }
    }
  }
	}}]` + suffix,
		},
		{
			name: "enumerate all attribute types",
			event: func(ctx context.Context) event.Event {
				return event.Event{
					At:      at,
					Message: "cache miss",
					Tags: event.TagList{
						event.TagOf("1_db", "godb"),

						event.TagOf("2a_age", 0.456), // Constant converted into "float64"
						event.TagOf("2b_ttl", float32(5000)),
						event.TagOf("2c_expiry_ms", float64(1e3)),

						event.TagOf("3a_retry", false),
						event.TagOf("3b_stale", true),

						event.TagOf("4a_max", 0x7fff), // Constant converted into "int"
						event.TagOf("4b_opcode", int8(0x7e)),
						event.TagOf("4c_base", int16(1<<9)),
						event.TagOf("4e_checksum", int32(0x11f7e294)),
						event.TagOf("4f_mode", int64(0644)),

						event.TagOf("5a_min", uint(1)),
						event.TagOf("5b_mix", uint8(44)),
						event.TagOf("5c_port", uint16(55678)),
						event.TagOf("5d_min_hops", uint32(1<<9)),
						event.TagOf("5e_max_hops", uint64(0xffffff)),
					},
				}
			},
			want: prefix + `"timeEvent":[{"time":"1970-01-01T00:00:40Z","annotation":{
  "description": { "value": "cache miss" },
  "attributes": {
    "attributeMap": {
      "1_db": { "stringValue": { "value": "godb" } },
      "2a_age": { "doubleValue": 0.456 },
      "2b_ttl": { "doubleValue": 5000 },
      "2c_expiry_ms": { "doubleValue": 1000 },
      "3a_retry": {},
			"3b_stale": { "boolValue": true },
      "4a_max": { "intValue": 32767 },
      "4b_opcode": { "intValue": 126 },
      "4c_base": { "intValue": 512 },
      "4e_checksum": { "intValue": 301458068 },
      "4f_mode": { "intValue": 420 },
      "5a_min": { "intValue": 1 },
      "5b_mix": { "intValue": 44 },
      "5c_port": { "intValue": 55678 },
      "5d_min_hops": { "intValue": 512 },
      "5e_max_hops": { "intValue": 16777215 }
    }
  }
}}]` + suffix,
		},
	}
	ctx := context.TODO()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startEvent := event.Event{
				Type:    event.StartSpanType,
				Message: "event span",
				At:      start,
			}
			endEvent := event.Event{
				Type: event.EndSpanType,
				At:   end,
			}
			ctx, _ := export.ContextSpan(ctx, startEvent)
			span := export.GetSpan(ctx)
			span.ID = export.SpanContext{}
			span.Events = []event.Event{tt.event(ctx)}
			exporter.ProcessEvent(ctx, startEvent)
			export.ContextSpan(ctx, endEvent)
			exporter.ProcessEvent(ctx, endEvent)
			exporter.Flush()
			got := sent.get("/v1/trace")
			checkJSON(t, got, []byte(tt.want))
		})
	}
}
