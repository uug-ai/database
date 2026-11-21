package database

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
)

func mustRaw(t *testing.T, v any) bson.Raw {
	t.Helper()
	raw, err := bson.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

func started(t *testing.T, conn string, req int64, db, name string, cmd bson.D) *event.CommandStartedEvent {
	t.Helper()
	return &event.CommandStartedEvent{
		Command:      mustRaw(t, cmd),
		DatabaseName: db,
		CommandName:  name,
		RequestID:    req,
		ConnectionID: conn,
	}
}

func finished(conn string, req int64, db, name string, d time.Duration) event.CommandFinishedEvent {
	return event.CommandFinishedEvent{
		CommandName:  name,
		DatabaseName: db,
		RequestID:    req,
		ConnectionID: conn,
		Duration:     d,
	}
}

func counter(t *testing.T, vec *prometheus.CounterVec, labels ...string) float64 {
	t.Helper()
	return testutil.ToFloat64(vec.WithLabelValues(labels...))
}

func TestCommandMetricsFindReply(t *testing.T) {
	metrics := newCommandMetrics(prometheus.NewRegistry())

	var forwarded []string
	next := &event.CommandMonitor{
		Started:   func(context.Context, *event.CommandStartedEvent) { forwarded = append(forwarded, "started") },
		Succeeded: func(context.Context, *event.CommandSucceededEvent) { forwarded = append(forwarded, "succeeded") },
		Failed:    func(context.Context, *event.CommandFailedEvent) { forwarded = append(forwarded, "failed") },
	}
	impl := &commandMetricsMonitor{metrics: metrics, app: "hub-api", next: next}
	monitor := impl.eventMonitor()
	ctx := context.Background()

	monitor.Started(ctx, started(t, "conn-1", 7, "Kerberos", "find",
		bson.D{{Key: "find", Value: "devices"}, {Key: "filter", Value: bson.D{{Key: "user_id", Value: "u"}}}}))
	reply := mustRaw(t, bson.D{
		{Key: "cursor", Value: bson.D{
			{Key: "firstBatch", Value: bson.A{bson.D{{Key: "key", Value: "a"}}, bson.D{{Key: "key", Value: "b"}}}},
			{Key: "id", Value: int64(0)},
			{Key: "ns", Value: "Kerberos.devices"},
		}},
		{Key: "ok", Value: 1.0},
	})
	monitor.Succeeded(ctx, &event.CommandSucceededEvent{
		CommandFinishedEvent: finished("conn-1", 7, "Kerberos", "find", 250*time.Millisecond),
		Reply:                reply,
	})

	labels := []string{"hub-api", "Kerberos", "devices", "find"}
	if got := counter(t, metrics.commands, labels...); got != 1 {
		t.Errorf("commands = %v, want 1", got)
	}
	if got, want := counter(t, metrics.replyB, labels...), float64(len(reply)); got != want {
		t.Errorf("reply bytes = %v, want %v", got, want)
	}
	if got := counter(t, metrics.replyDocs, labels...); got != 2 {
		t.Errorf("reply documents = %v, want 2", got)
	}
	if got := counter(t, metrics.duration, labels...); got != 0.25 {
		t.Errorf("duration = %v, want 0.25", got)
	}
	if got := counter(t, metrics.failures, labels...); got != 0 {
		t.Errorf("failures = %v, want 0", got)
	}
	if len(forwarded) != 2 || forwarded[0] != "started" || forwarded[1] != "succeeded" {
		t.Errorf("forwarded events = %v, want [started succeeded]", forwarded)
	}
	if _, ok := impl.inflight.Load(inflightKey{"conn-1", 7}); ok {
		t.Error("in-flight entry not removed after the command finished")
	}
}

func TestCommandMetricsGetMoreUsesCollectionField(t *testing.T) {
	metrics := newCommandMetrics(prometheus.NewRegistry())
	monitor := newCommandMetricsMonitor(metrics, "vault", nil)
	ctx := context.Background()

	monitor.Started(ctx, started(t, "conn-2", 9, "KerberosStorage", "getMore",
		bson.D{{Key: "getMore", Value: int64(42)}, {Key: "collection", Value: "media"}}))
	reply := mustRaw(t, bson.D{
		{Key: "cursor", Value: bson.D{
			{Key: "nextBatch", Value: bson.A{bson.D{}, bson.D{}, bson.D{}}},
			{Key: "id", Value: int64(42)},
		}},
		{Key: "ok", Value: 1.0},
	})
	monitor.Succeeded(ctx, &event.CommandSucceededEvent{
		CommandFinishedEvent: finished("conn-2", 9, "KerberosStorage", "getMore", time.Millisecond),
		Reply:                reply,
	})

	labels := []string{"vault", "KerberosStorage", "media", "getMore"}
	if got, want := counter(t, metrics.replyB, labels...), float64(len(reply)); got != want {
		t.Errorf("reply bytes = %v, want %v", got, want)
	}
	if got := counter(t, metrics.replyDocs, labels...); got != 3 {
		t.Errorf("reply documents = %v, want 3", got)
	}
}

func TestCommandMetricsFailure(t *testing.T) {
	metrics := newCommandMetrics(prometheus.NewRegistry())
	monitor := newCommandMetricsMonitor(metrics, "hub-api", nil)
	ctx := context.Background()

	monitor.Started(ctx, started(t, "conn-3", 1, "Kerberos", "update",
		bson.D{{Key: "update", Value: "users"}}))
	monitor.Failed(ctx, &event.CommandFailedEvent{
		CommandFinishedEvent: finished("conn-3", 1, "Kerberos", "update", time.Second),
		Failure:              "boom",
	})

	labels := []string{"hub-api", "Kerberos", "users", "update"}
	if got := counter(t, metrics.commands, labels...); got != 1 {
		t.Errorf("commands = %v, want 1", got)
	}
	if got := counter(t, metrics.failures, labels...); got != 1 {
		t.Errorf("failures = %v, want 1", got)
	}
	if got := testutil.CollectAndCount(metrics.replyB); got != 0 {
		t.Errorf("reply byte series = %d, want 0 after a failure", got)
	}
}

func TestCommandCollection(t *testing.T) {
	tests := []struct {
		name    string
		command string
		cmd     bson.Raw
		want    string
	}{
		{"find", "find", mustRaw(t, bson.D{{Key: "find", Value: "media"}}), "media"},
		{"aggregate on collection", "aggregate", mustRaw(t, bson.D{{Key: "aggregate", Value: "analysis"}}), "analysis"},
		{"database aggregate", "aggregate", mustRaw(t, bson.D{{Key: "aggregate", Value: 1}}), ""},
		{"ping", "ping", mustRaw(t, bson.D{{Key: "ping", Value: 1}}), ""},
		{"getMore", "getMore", mustRaw(t, bson.D{{Key: "getMore", Value: int64(1)}, {Key: "collection", Value: "users"}}), "users"},
		{"redacted", "saslStart", nil, ""},
		{"name mismatch", "find", mustRaw(t, bson.D{{Key: "$db", Value: "x"}}), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commandCollection(tt.command, tt.cmd); got != tt.want {
				t.Errorf("commandCollection() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommandMetricsNoCollectionLabel(t *testing.T) {
	metrics := newCommandMetrics(prometheus.NewRegistry())
	monitor := newCommandMetricsMonitor(metrics, "hub-api", nil)
	ctx := context.Background()

	// A finish without a matching start (or a redacted command) still counts.
	reply := mustRaw(t, bson.D{{Key: "ok", Value: 1.0}})
	monitor.Succeeded(ctx, &event.CommandSucceededEvent{
		CommandFinishedEvent: finished("conn-4", 3, "admin", "ping", time.Millisecond),
		Reply:                reply,
	})

	labels := []string{"hub-api", "admin", "", "ping"}
	if got, want := counter(t, metrics.replyB, labels...), float64(len(reply)); got != want {
		t.Errorf("reply bytes = %v, want %v", got, want)
	}
	if got := testutil.CollectAndCount(metrics.replyDocs); got != 0 {
		t.Errorf("reply document series = %d, want 0 for a reply without a cursor", got)
	}
}

func TestCommandMetricsSeriesBound(t *testing.T) {
	metrics := newCommandMetrics(prometheus.NewRegistry())
	metrics.maxSeries = 2
	monitor := newCommandMetricsMonitor(metrics, "app", nil)
	ctx := context.Background()

	for i, coll := range []string{"a", "b", "c", "d", "a"} {
		req := int64(i)
		monitor.Started(ctx, started(t, "conn", req, "db", "find", bson.D{{Key: "find", Value: coll}}))
		monitor.Succeeded(ctx, &event.CommandSucceededEvent{
			CommandFinishedEvent: finished("conn", req, "db", "find", 0),
			Reply:                mustRaw(t, bson.D{{Key: "ok", Value: 1.0}}),
		})
	}

	if got := counter(t, metrics.commands, "app", "db", "a", "find"); got != 2 {
		t.Errorf("collection a = %v, want 2 (seen before the bound)", got)
	}
	if got := counter(t, metrics.commands, "app", "db", "b", "find"); got != 1 {
		t.Errorf("collection b = %v, want 1", got)
	}
	if got := counter(t, metrics.commands, "app", "db", otherCollection, "find"); got != 2 {
		t.Errorf("%s = %v, want 2 (c and d past the bound)", otherCollection, got)
	}
}

func TestRegisterCounterVecReusesExisting(t *testing.T) {
	reg := prometheus.NewRegistry()
	first := newCommandMetrics(reg)
	second := newCommandMetrics(reg)
	if first.commands != second.commands {
		t.Fatal("second registration did not reuse the registered collector")
	}
}

func TestBuildMongoClientOptionsInstallsMetricsMonitor(t *testing.T) {
	t.Run("URI keeps tracing and labels by app name", func(t *testing.T) {
		opts := NewMongoOptions().
			SetUri("mongodb://localhost:27017/?appName=hub-monitor-device").
			SetTimeout(5000).
			Build()

		clientOpts, err := buildMongoClientOptions(opts)
		if err != nil {
			t.Fatalf("build client options: %v", err)
		}
		if clientOpts.Monitor == nil || clientOpts.Monitor.Started == nil || clientOpts.Monitor.Succeeded == nil {
			t.Fatal("expected a command monitor")
		}

		ctx := context.Background()
		clientOpts.Monitor.Started(ctx, started(t, "conn-uri", 1, "Kerberos", "find",
			bson.D{{Key: "find", Value: "subscriptions"}}))
		reply := mustRaw(t, bson.D{{Key: "ok", Value: 1.0}})
		clientOpts.Monitor.Succeeded(ctx, &event.CommandSucceededEvent{
			CommandFinishedEvent: finished("conn-uri", 1, "Kerberos", "find", time.Millisecond),
			Reply:                reply,
		})

		got := counter(t, sharedCommandMetrics().replyB, "hub-monitor-device", "Kerberos", "subscriptions", "find")
		if got != float64(len(reply)) {
			t.Errorf("reply bytes = %v, want %v", got, len(reply))
		}
	})

	t.Run("Components", func(t *testing.T) {
		opts := NewMongoOptions().
			SetHost("localhost:27017").
			SetAuthSource("admin").
			SetUsername("user").
			SetPassword("password").
			SetTimeout(5000).
			Build()

		clientOpts, err := buildMongoClientOptions(opts)
		if err != nil {
			t.Fatalf("build client options: %v", err)
		}
		if clientOpts.Monitor == nil || clientOpts.Monitor.Succeeded == nil {
			t.Fatal("expected a command monitor")
		}
	})
}
