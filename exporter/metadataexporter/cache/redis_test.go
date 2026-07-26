package metadataexporter

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hanzokv/go/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pipeline"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// buildRedisKeyCache wires a cache to an in-process Redis. miniredis speaks the
// real protocol, so the cache issues the same SADD/SCARD/SMISMEMBER/PFADD/
// PFCOUNT/EXPIRE/KEYS it issues in production — including through a pipeline.
func buildRedisKeyCache(t *testing.T) (*RedisKeyCache, *miniredis.Miniredis) {
	t.Helper()
	srv := miniredis.RunT(t)

	cache := &RedisKeyCache{
		redisClient: kv.NewClient(&kv.Options{Addr: srv.Addr()}),
		tenantID:    "testTenant",
		logger:      zap.NewNop(),

		tracesTTL:  10 * time.Second,
		metricsTTL: 20 * time.Second,
		logsTTL:    30 * time.Second,

		maxTracesResourceFp:              2,
		maxMetricsResourceFp:             5,
		maxLogsResourceFp:                5,
		maxTracesCardinalityPerResource:  3,
		maxMetricsCardinalityPerResource: 10,
		maxLogsCardinalityPerResource:    10,
		tracesMaxTotalCardinality:        20,
		metricsMaxTotalCardinality:       20,
		logsMaxTotalCardinality:          20,
	}
	t.Cleanup(func() { require.NoError(t, cache.Close(context.Background())) })
	return cache, srv
}

// seedHLL registers n distinct members in the HyperLogLog at key, so PFCount
// reports n.
func seedHLL(t *testing.T, c *RedisKeyCache, key string, n int) {
	t.Helper()
	members := make([]any, n)
	for i := range members {
		members[i] = "seed-" + strconv.Itoa(i)
	}
	require.NoError(t, c.redisClient.PFAdd(context.Background(), key, members...).Err())
	require.Equal(t, int64(n), c.redisClient.PFCount(context.Background(), key).Val())
}

func TestNewRedisKeyCache(t *testing.T) {
	srv := miniredis.RunT(t)
	addr := srv.Addr()

	cache, err := NewRedisKeyCache(RedisKeyCacheOptions{
		Addr:     addr,
		TenantID: "testTenant",
		Logger:   zap.NewNop(),
	})
	require.NoError(t, err)
	require.NoError(t, cache.Close(context.Background()))

	// The constructor pings, so a dead server must surface as an error rather
	// than a cache that fails on first use.
	srv.Close()
	_, err = NewRedisKeyCache(RedisKeyCacheOptions{Addr: addr, Logger: zap.NewNop()})
	require.Error(t, err)
}

func TestRedisKeyCache_AddAttrsToResource_NewResource_Success(t *testing.T) {
	ctx := context.Background()
	cache, srv := buildRedisKeyCache(t)

	epochWindow := getCurrentEpochWindowMillis()
	resourceHLLKey := fmt.Sprintf("testTenant:metadata:traces:%d:resources:hll", epochWindow)
	attrsKey := fmt.Sprintf("testTenant:metadata:traces:%d:resource:1000", epochWindow)
	attrsHLLKey := fmt.Sprintf("testTenant:metadata:traces:%d:attrs:hll", epochWindow)

	require.NoError(t, cache.AddAttrsToResource(ctx, 1000, []uint64{1, 2}, pipeline.SignalTraces))

	members, err := srv.Members(attrsKey)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"1", "2"}, members)

	assert.Equal(t, int64(1), cache.redisClient.PFCount(ctx, resourceHLLKey).Val())
	assert.Equal(t, int64(2), cache.redisClient.PFCount(ctx, attrsHLLKey).Val())

	// The traces TTL is applied to every key the pipeline touches.
	for _, key := range []string{resourceHLLKey, attrsKey, attrsHLLKey} {
		assert.Equal(t, 10*time.Second, srv.TTL(key), "ttl on %s", key)
	}
}

func TestRedisKeyCache_AddAttrsToResource_ResourceLimitExceeded(t *testing.T) {
	ctx := context.Background()
	cache, srv := buildRedisKeyCache(t)

	epochWindow := getCurrentEpochWindowMillis()
	resourceHLLKey := fmt.Sprintf("testTenant:metadata:traces:%d:resources:hll", epochWindow)
	attrsKey := fmt.Sprintf("testTenant:metadata:traces:%d:resource:2000", epochWindow)

	// maxTracesResourceFp is 2, so two known resources already fills the window.
	seedHLL(t, cache, resourceHLLKey, 2)

	err := cache.AddAttrsToResource(ctx, 2000, []uint64{123}, pipeline.SignalTraces)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many resource fingerprints")
	assert.False(t, srv.Exists(attrsKey), "rejected resource must not be written")
}

func TestRedisKeyCache_AddAttrsToResource_AttrCardinalityExceeded(t *testing.T) {
	ctx := context.Background()
	cache, srv := buildRedisKeyCache(t)

	epochWindow := getCurrentEpochWindowMillis()
	resourceHLLKey := fmt.Sprintf("testTenant:metadata:traces:%d:resources:hll", epochWindow)
	attrsKey := fmt.Sprintf("testTenant:metadata:traces:%d:resource:3000", epochWindow)

	seedHLL(t, cache, resourceHLLKey, 1) // under the resource limit
	_, err := srv.SetAdd(attrsKey, "1", "2", "3")
	require.NoError(t, err)

	// maxTracesCardinalityPerResource is 3 and the resource already holds 3.
	err = cache.AddAttrsToResource(ctx, 3000, []uint64{99}, pipeline.SignalTraces)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many attribute fingerprints")

	members, err := srv.Members(attrsKey)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"1", "2", "3"}, members, "rejected attrs must not be written")
}

func TestRedisKeyCache_AddAttrsToResource_EmptyList(t *testing.T) {
	ctx := context.Background()
	cache, srv := buildRedisKeyCache(t)

	require.NoError(t, cache.AddAttrsToResource(ctx, 9999, nil, pipeline.SignalTraces))
	require.NoError(t, cache.AddAttrsToResource(ctx, 9999, []uint64{}, pipeline.SignalTraces))

	assert.Empty(t, srv.Keys(), "an empty attr list must issue no commands")
}

func TestRedisKeyCache_AttrsExistForResource_Basic(t *testing.T) {
	ctx := context.Background()
	cache, srv := buildRedisKeyCache(t)

	epochWindow := getCurrentEpochWindowMillis()
	attrsKey := fmt.Sprintf("testTenant:metadata:metrics:%d:resource:5555", epochWindow)
	_, err := srv.SetAdd(attrsKey, "10", "30")
	require.NoError(t, err)

	exists, err := cache.AttrsExistForResource(ctx, 5555, []uint64{10, 20, 30}, pipeline.SignalMetrics)
	require.NoError(t, err)
	assert.Equal(t, []bool{true, false, true}, exists)
}

func TestRedisKeyCache_AttrsExistForResource_Empty(t *testing.T) {
	cache, _ := buildRedisKeyCache(t)

	exists, err := cache.AttrsExistForResource(context.Background(), 1234, []uint64{}, pipeline.SignalLogs)
	require.NoError(t, err)
	assert.Nil(t, exists)
}

func TestRedisKeyCache_ResourcesLimitExceeded(t *testing.T) {
	ctx := context.Background()
	cache, _ := buildRedisKeyCache(t)

	resourceHLLKey := fmt.Sprintf("testTenant:metadata:traces:%d:resources:hll", getCurrentEpochWindowMillis())

	assert.False(t, cache.ResourcesLimitExceeded(ctx, pipeline.SignalTraces))
	seedHLL(t, cache, resourceHLLKey, 3) // maxTracesResourceFp is 2
	assert.True(t, cache.ResourcesLimitExceeded(ctx, pipeline.SignalTraces))
	assert.False(t, cache.ResourcesLimitExceeded(ctx, pipeline.SignalMetrics), "limits are per signal")
}

func TestRedisKeyCache_TotalCardinalityLimitExceeded(t *testing.T) {
	ctx := context.Background()
	cache, _ := buildRedisKeyCache(t)

	attrsHLLKey := fmt.Sprintf("testTenant:metadata:traces:%d:attrs:hll", getCurrentEpochWindowMillis())

	assert.False(t, cache.TotalCardinalityLimitExceeded(ctx, pipeline.SignalTraces))
	seedHLL(t, cache, attrsHLLKey, 20) // tracesMaxTotalCardinality is 20
	assert.True(t, cache.TotalCardinalityLimitExceeded(ctx, pipeline.SignalTraces))
}

func TestRedisKeyCache_CardinalityLimitExceeded(t *testing.T) {
	ctx := context.Background()
	cache, srv := buildRedisKeyCache(t)

	attrsKey := fmt.Sprintf("testTenant:metadata:traces:%d:resource:777", getCurrentEpochWindowMillis())

	assert.False(t, cache.CardinalityLimitExceeded(ctx, 777, pipeline.SignalTraces))
	_, err := srv.SetAdd(attrsKey, "1", "2", "3") // maxTracesCardinalityPerResource is 3
	require.NoError(t, err)
	assert.True(t, cache.CardinalityLimitExceeded(ctx, 777, pipeline.SignalTraces))
}

// TestRedisKeyCache_CardinalityLimitExceededMulti covers the pipelined path,
// which reads its results back through a *kv.IntCmd type assertion.
func TestRedisKeyCache_CardinalityLimitExceededMulti(t *testing.T) {
	ctx := context.Background()
	cache, srv := buildRedisKeyCache(t)

	epochWindow := getCurrentEpochWindowMillis()
	full := fmt.Sprintf("testTenant:metadata:traces:%d:resource:1", epochWindow)
	partial := fmt.Sprintf("testTenant:metadata:traces:%d:resource:2", epochWindow)

	_, err := srv.SetAdd(full, "1", "2", "3")
	require.NoError(t, err)
	_, err = srv.SetAdd(partial, "1")
	require.NoError(t, err)

	out, err := cache.CardinalityLimitExceededMulti(ctx, []uint64{1, 2, 3}, pipeline.SignalTraces)
	require.NoError(t, err)
	assert.Equal(t, []bool{true, false, false}, out)
}

func TestRedisKeyCache_Debug(t *testing.T) {
	ctx := context.Background()
	cache, srv := buildRedisKeyCache(t)

	core, logs := observer.New(zap.DebugLevel)
	cache.logger = zap.New(core)
	cache.debug = true

	epochWindow := getCurrentEpochWindowMillis()
	attrsKey := fmt.Sprintf("testTenant:metadata:traces:%d:resource:1000", epochWindow)
	attrsHLLKey := fmt.Sprintf("testTenant:metadata:traces:%d:attrs:hll", epochWindow)

	_, err := srv.SetAdd(attrsKey, "1", "2")
	require.NoError(t, err)
	seedHLL(t, cache, attrsHLLKey, 4)

	cache.Debug(ctx)

	assert.Equal(t, 1, logs.FilterMessage("set cardinality").Len())
	assert.Equal(t, 1, logs.FilterMessage("HLL cardinality").Len())

	// debug=false is the production default and must stay silent.
	cache.debug = false
	before := logs.Len()
	cache.Debug(ctx)
	assert.Equal(t, before, logs.Len())
}
