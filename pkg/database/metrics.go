package database

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
)

// Command metrics count what each process asks of MongoDB and how many bytes
// come back, so database egress can be attributed to the service, database,
// collection and command that caused it. They are registered with the
// Prometheus default registerer and served by any promhttp.Handler().
//
// Reply bytes are the size of the uncompressed BSON reply the driver
// received: the logical response payload. They exclude TLS and wire-protocol
// overhead and are larger than the wire bytes when compression is negotiated.
// Every cursor batch (find/aggregate first batch and each getMore) is counted.
// No document contents are inspected beyond the cursor batch length.
const (
	metricsLabelApp        = "app"
	metricsLabelDatabase   = "database"
	metricsLabelCollection = "collection"
	metricsLabelCommand    = "command"

	// otherCollection replaces the collection label once a process has seen
	// maxMetricSeries distinct label sets, bounding Prometheus cardinality
	// if a caller ever builds collection names dynamically.
	otherCollection = "_other"
	maxMetricSeries = 2000
)

var commandMetricLabels = []string{
	metricsLabelApp,
	metricsLabelDatabase,
	metricsLabelCollection,
	metricsLabelCommand,
}

type commandMetrics struct {
	commands  *prometheus.CounterVec
	failures  *prometheus.CounterVec
	replyB    *prometheus.CounterVec
	replyDocs *prometheus.CounterVec
	duration  *prometheus.CounterVec

	seen      sync.Map // commandLabels -> struct{}
	seenCount atomic.Int64
	maxSeries int64
}

type commandLabels struct {
	app        string
	database   string
	collection string
	command    string
}

func (l commandLabels) values() []string {
	return []string{l.app, l.database, l.collection, l.command}
}

var (
	defaultCommandMetricsOnce sync.Once
	defaultCommandMetrics     *commandMetrics
)

// sharedCommandMetrics returns the process-wide metrics registered with the
// Prometheus default registerer. Every client in the process shares them; the
// app label tells clients with different application names apart.
func sharedCommandMetrics() *commandMetrics {
	defaultCommandMetricsOnce.Do(func() {
		defaultCommandMetrics = newCommandMetrics(prometheus.DefaultRegisterer)
	})
	return defaultCommandMetrics
}

func newCommandMetrics(reg prometheus.Registerer) *commandMetrics {
	return &commandMetrics{
		commands: registerCounterVec(reg, prometheus.CounterOpts{
			Name: "mongodb_client_commands_total",
			Help: "MongoDB commands completed by this process, successful or failed.",
		}),
		failures: registerCounterVec(reg, prometheus.CounterOpts{
			Name: "mongodb_client_command_failures_total",
			Help: "MongoDB commands that returned an error.",
		}),
		replyB: registerCounterVec(reg, prometheus.CounterOpts{
			Name: "mongodb_client_reply_bytes_total",
			Help: "Uncompressed BSON bytes of successful MongoDB command replies (logical response payload).",
		}),
		replyDocs: registerCounterVec(reg, prometheus.CounterOpts{
			Name: "mongodb_client_reply_documents_total",
			Help: "Documents returned in MongoDB cursor batches (find, aggregate and getMore).",
		}),
		duration: registerCounterVec(reg, prometheus.CounterOpts{
			Name: "mongodb_client_command_duration_seconds_total",
			Help: "Total time spent in MongoDB commands, as measured by the driver.",
		}),
		maxSeries: maxMetricSeries,
	}
}

// registerCounterVec registers a counter vector, reusing an identical
// collector that is already registered (for example by a second copy of this
// package linked into the same binary) instead of panicking.
func registerCounterVec(reg prometheus.Registerer, opts prometheus.CounterOpts) *prometheus.CounterVec {
	vec := prometheus.NewCounterVec(opts, commandMetricLabels)
	if err := reg.Register(vec); err != nil {
		var already prometheus.AlreadyRegisteredError
		if errors.As(err, &already) {
			if existing, ok := already.ExistingCollector.(*prometheus.CounterVec); ok {
				return existing
			}
		}
		// A conflicting collector with the same name but another shape is
		// a programming error in the host service; keep the vector
		// unregistered so database calls still work.
	}
	return vec
}

// bound returns labels whose series count stays within maxSeries.
func (m *commandMetrics) bound(l commandLabels) commandLabels {
	if _, ok := m.seen.Load(l); ok {
		return l
	}
	if m.seenCount.Load() >= m.maxSeries {
		l.collection = otherCollection
		return l
	}
	if _, loaded := m.seen.LoadOrStore(l, struct{}{}); !loaded {
		m.seenCount.Add(1)
	}
	return l
}

type inflightKey struct {
	connectionID string
	requestID    int64
}

type commandMetricsMonitor struct {
	metrics  *commandMetrics
	app      string
	inflight sync.Map // inflightKey -> collection name
	next     *event.CommandMonitor
}

// NewCommandMetricsMonitor returns a command monitor that records Prometheus
// command, failure, reply-byte, reply-document and duration counters and then
// forwards every event to next, which may be nil. Use it when building a
// mongo.Client outside this package:
//
//	opts.SetMonitor(database.NewCommandMetricsMonitor("hub-api",
//		otelmongo.NewMonitor()))
//
// appName becomes the app label; pass the application name reported to the
// server. Clients created with NewMongoClient install this monitor already.
func NewCommandMetricsMonitor(appName string, next *event.CommandMonitor) *event.CommandMonitor {
	return newCommandMetricsMonitor(sharedCommandMetrics(), appName, next)
}

func newCommandMetricsMonitor(metrics *commandMetrics, appName string, next *event.CommandMonitor) *event.CommandMonitor {
	m := &commandMetricsMonitor{metrics: metrics, app: appName, next: next}
	return m.eventMonitor()
}

func (m *commandMetricsMonitor) eventMonitor() *event.CommandMonitor {
	return &event.CommandMonitor{
		Started:   m.started,
		Succeeded: m.succeeded,
		Failed:    m.failed,
	}
}

func (m *commandMetricsMonitor) started(ctx context.Context, evt *event.CommandStartedEvent) {
	if collection := commandCollection(evt.CommandName, evt.Command); collection != "" {
		m.inflight.Store(inflightKey{evt.ConnectionID, evt.RequestID}, collection)
	}
	if m.next != nil && m.next.Started != nil {
		m.next.Started(ctx, evt)
	}
}

func (m *commandMetricsMonitor) succeeded(ctx context.Context, evt *event.CommandSucceededEvent) {
	labels := m.finish(&evt.CommandFinishedEvent)
	m.metrics.replyB.WithLabelValues(labels...).Add(float64(len(evt.Reply)))
	if docs := cursorBatchLength(evt.Reply); docs > 0 {
		m.metrics.replyDocs.WithLabelValues(labels...).Add(float64(docs))
	}
	if m.next != nil && m.next.Succeeded != nil {
		m.next.Succeeded(ctx, evt)
	}
}

func (m *commandMetricsMonitor) failed(ctx context.Context, evt *event.CommandFailedEvent) {
	labels := m.finish(&evt.CommandFinishedEvent)
	m.metrics.failures.WithLabelValues(labels...).Inc()
	if m.next != nil && m.next.Failed != nil {
		m.next.Failed(ctx, evt)
	}
}

// finish records the counters common to success and failure and returns the
// label values for the command.
func (m *commandMetricsMonitor) finish(evt *event.CommandFinishedEvent) []string {
	var collection string
	if v, ok := m.inflight.LoadAndDelete(inflightKey{evt.ConnectionID, evt.RequestID}); ok {
		collection = v.(string)
	}
	labels := m.metrics.bound(commandLabels{
		app:        m.app,
		database:   evt.DatabaseName,
		collection: collection,
		command:    evt.CommandName,
	}).values()
	m.metrics.commands.WithLabelValues(labels...).Inc()
	m.metrics.duration.WithLabelValues(labels...).Add(evt.Duration.Seconds())
	return labels
}

// commandCollection returns the collection a command targets, or "" for
// database-level commands (ping, hello, aggregate: 1, ...) and for commands
// the driver redacted from monitoring.
func commandCollection(name string, cmd bson.Raw) string {
	if len(cmd) == 0 {
		return ""
	}
	if name == "getMore" {
		if v, err := cmd.LookupErr("collection"); err == nil {
			if s, ok := v.StringValueOK(); ok {
				return s
			}
		}
		return ""
	}
	first, err := cmd.IndexErr(0)
	if err != nil || first.Key() != name {
		return ""
	}
	if s, ok := first.Value().StringValueOK(); ok {
		return s
	}
	return ""
}

// cursorBatchLength returns the number of documents in a cursor reply's
// firstBatch or nextBatch, or 0 when the reply carries no cursor.
func cursorBatchLength(reply bson.Raw) int {
	if len(reply) == 0 {
		return 0
	}
	cursor, err := reply.LookupErr("cursor")
	if err != nil {
		return 0
	}
	doc, ok := cursor.DocumentOK()
	if !ok {
		return 0
	}
	for _, key := range []string{"firstBatch", "nextBatch"} {
		batch, err := doc.LookupErr(key)
		if err != nil {
			continue
		}
		arr, ok := batch.ArrayOK()
		if !ok {
			return 0
		}
		values, err := arr.Values()
		if err != nil {
			return 0
		}
		return len(values)
	}
	return 0
}
