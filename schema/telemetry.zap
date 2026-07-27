# Hanzo Telemetry — ZAP Schema
#
# The wire Hanzo services use to ship traces, logs and metrics. It replaces
# OTLP: the payload is ZAP the whole way down rather than protobuf carried
# inside a ZAP frame, and nothing here reaches for gRPC.
#
# Signals land in hanzoai/datastore.
#
# Code generation:
#   zapc generate schema/telemetry.zap --lang go --out ./gen/zap/
#   zapc generate schema/telemetry.zap --lang ts --out ./gen/zap/

# ── Time ─────────────────────────────────────────────────────────────────
#
# Nanoseconds since the Unix epoch, as Int64 — the range runs to 2262.
# Trace and span ids are lowercase hex text, so a log line stays readable.

# ── Attributes ───────────────────────────────────────────────────────────
#
# An attribute value is one of a few scalar shapes. Rather than a union, a
# Value carries a kind and the field that kind names — the reader checks
# kind and takes one field, and an unset field costs nothing on the wire.

enum Kind
  empty
  text
  int
  float
  bool
  bytes
  list

struct Value
  kind   Kind
  text   Text
  int    Int64
  float  Float64
  bool   Bool
  bytes  Text   # base64
  list   List(Value)

struct Attr
  key   Text
  value Value

# ── Origin ───────────────────────────────────────────────────────────────
#
# Resource is what produced a signal — a service, a pod, a host. Scope is
# the instrumentation inside it that emitted the signal.

struct Resource
  attrs   List(Attr)
  dropped Int32

struct Scope
  name    Text
  version Text
  attrs   List(Attr)
  dropped Int32

# ── Traces ───────────────────────────────────────────────────────────────

enum SpanKind
  unspecified
  internal
  server
  client
  producer
  consumer

enum Status
  unset
  ok
  error

struct Event
  time    Int64
  name    Text
  attrs   List(Attr)
  dropped Int32

struct Link
  trace   Text
  span    Text
  state   Text
  attrs   List(Attr)
  dropped Int32

struct Span
  trace   Text
  span    Text
  parent  Text
  state   Text
  flags   Int32
  name    Text
  kind    SpanKind
  start   Int64
  end     Int64
  attrs   List(Attr)
  dropped Int32
  events  List(Event)
  links   List(Link)
  status  Status
  message Text

struct Trace
  resource Resource
  scope    Scope
  spans    List(Span)
  schema   Text

# ── Logs ─────────────────────────────────────────────────────────────────

enum Severity
  unspecified
  trace
  debug
  info
  warn
  error
  fatal

struct Log
  time     Int64
  observed Int64
  severity Severity
  level    Text
  body     Value
  attrs    List(Attr)
  dropped  Int32
  flags    Int32
  trace    Text
  span     Text

struct Logs
  resource Resource
  scope    Scope
  records  List(Log)
  schema   Text

# ── Metrics ──────────────────────────────────────────────────────────────
#
# A point carries its own attributes and window. Sum and Histogram add the
# aggregation facts a reader needs to interpret the numbers.

enum Temporality
  unspecified
  delta
  cumulative

struct Point
  attrs   List(Attr)
  start   Int64
  time    Int64
  int     Int64
  float   Float64
  isFloat Bool
  flags   Int32

struct Bucket
  bound Int64
  count Int64

struct Distribution
  attrs   List(Attr)
  start   Int64
  time    Int64
  count   Int64
  sum     Float64
  min     Float64
  max     Float64
  buckets List(Bucket)
  flags   Int32

struct Gauge
  points List(Point)

struct Sum
  points      List(Point)
  temporality Temporality
  monotonic   Bool

struct Histogram
  points      List(Distribution)
  temporality Temporality

enum Shape
  gauge
  sum
  histogram

struct Metric
  name      Text
  unit      Text
  note      Text
  shape     Shape
  gauge     Gauge
  sum       Sum
  histogram Histogram
  attrs     List(Attr)

struct Metrics
  resource Resource
  scope    Scope
  metrics  List(Metric)
  schema   Text

# ── Wire ─────────────────────────────────────────────────────────────────
#
# One signal per call. A batch groups signals sharing a resource and scope,
# which is how they arrive from a collector pipeline.

struct TraceRequest
  batches List(Trace)

struct LogRequest
  batches List(Logs)

struct MetricRequest
  batches List(Metrics)

# A reply names what the receiver refused and why. Empty means it took
# everything.

struct Refused
  count  Int64
  reason Text

struct Reply
  refused Refused

# ── Service interface ────────────────────────────────────────────────────

interface Telemetry
  # Ship spans. Mounted at POST /v1/traces.
  traces (request TraceRequest) -> (response Reply)

  # Ship log records. Mounted at POST /v1/logs.
  logs (request LogRequest) -> (response Reply)

  # Ship metric points. Mounted at POST /v1/metrics.
  metrics (request MetricRequest) -> (response Reply)
