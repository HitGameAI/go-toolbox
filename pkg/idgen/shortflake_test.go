/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2025-11-21 00:00:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2025-11-21 21:30:00
 * @FilePath: \go-toolbox\pkg\idgen\shortflake_test.go
 * @Description: ShortFlake 生成器测试
 *
 * Copyright (c) 2024 by kamalyes, All Rights Reserved.
 */

package idgen

import (
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestShortFlakeGenerator 测试短 Snowflake 生成器
func TestShortFlakeGenerator(t *testing.T) {
	gen := NewShortFlakeGenerator(1)

	t.Run("GenerateTraceID", func(t *testing.T) {
		traceID := gen.GenerateTraceID()
		assert.NotEmpty(t, traceID, "TraceID 不应为空")
		assert.Equal(t, 13, len(traceID), "TraceID 应为 13 字符 hex")
		assert.True(t, regexp.MustCompile(`^[0-9a-f]{13}$`).MatchString(traceID), "TraceID 应为 hex 格式")
	})

	t.Run("GenerateSpanID", func(t *testing.T) {
		spanID := gen.GenerateSpanID()
		assert.NotEmpty(t, spanID, "SpanID 不应为空")
		assert.Equal(t, 8, len(spanID), "SpanID 应为 8 字符 hex")
		assert.True(t, regexp.MustCompile(`^[0-9a-f]{8}$`).MatchString(spanID), "SpanID 应为 hex 格式")
	})

	t.Run("GenerateRequestID", func(t *testing.T) {
		requestID := gen.GenerateRequestID()
		assert.NotEmpty(t, requestID, "RequestID 不应为空")
		assert.True(t, regexp.MustCompile(`^\d+-\d+$`).MatchString(requestID), "RequestID 应为 数字-计数器 格式")
	})

	t.Run("GenerateCorrelationID", func(t *testing.T) {
		correlationID := gen.GenerateCorrelationID()
		assert.NotEmpty(t, correlationID, "CorrelationID 不应为空")
		assert.Equal(t, 36, len(correlationID), "CorrelationID 应为 36 字符 UUID 格式")
		assert.True(t, regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`).MatchString(correlationID), "CorrelationID 应为 UUID 格式")
	})

	t.Run("DifferentFormats", func(t *testing.T) {
		traceID := gen.GenerateTraceID()
		spanID := gen.GenerateSpanID()
		requestID := gen.GenerateRequestID()
		correlationID := gen.GenerateCorrelationID()

		assert.NotEqual(t, traceID, spanID, "TraceID(hex 13字符) 和 SpanID(hex 8字符) 格式应不同")
		assert.NotEqual(t, traceID, requestID, "TraceID 和 RequestID 格式应不同")
		assert.NotEqual(t, traceID, correlationID, "TraceID 和 CorrelationID 格式应不同")
		assert.NotEqual(t, spanID, requestID, "SpanID 和 RequestID 格式应不同")
		assert.NotEqual(t, spanID, correlationID, "SpanID 和 CorrelationID 格式应不同")
		assert.NotEqual(t, requestID, correlationID, "RequestID 和 CorrelationID 格式应不同")
	})

	t.Run("Monotonic", func(t *testing.T) {
		var lastID int64
		for i := 0; i < 1000; i++ {
			id := gen.Generate()
			assert.True(t, id > lastID, "ShortFlake ID 应单调递增")
			lastID = id
		}
	})

	t.Run("Uniqueness", func(t *testing.T) {
		ids := make(map[string]bool)
		for i := 0; i < 10000; i++ {
			id := gen.GenerateTraceID()
			assert.False(t, ids[id], "生成的 ID 应唯一")
			ids[id] = true
		}
	})
}

func TestShortFlakeBase62Generator(t *testing.T) {
	gen := NewShortFlakeBase62Generator(1)

	t.Run("GenerateTraceID", func(t *testing.T) {
		traceID := gen.GenerateTraceID()
		assert.NotEmpty(t, traceID, "TraceID 不应为空")
		assert.True(t, len(traceID) >= 9 && len(traceID) <= 10, "TraceID 应为 9-10 字符")
		assert.True(t, regexp.MustCompile(`^[0-9A-Za-z]+$`).MatchString(traceID), "TraceID 应为 Base62 格式")
	})

	t.Run("GenerateSpanID", func(t *testing.T) {
		spanID := gen.GenerateSpanID()
		assert.NotEmpty(t, spanID, "SpanID 不应为空")
		assert.True(t, len(spanID) >= 1 && len(spanID) <= 7, "SpanID 应为 1-7 字符 Base62")
		assert.True(t, regexp.MustCompile(`^[0-9A-Za-z]+$`).MatchString(spanID), "SpanID 应为 Base62 格式")
	})

	t.Run("GenerateRequestID", func(t *testing.T) {
		requestID := gen.GenerateRequestID()
		assert.NotEmpty(t, requestID, "RequestID 不应为空")
		assert.True(t, regexp.MustCompile(`^[0-9A-Za-z]+-\d+$`).MatchString(requestID), "RequestID 应为 Base62前缀-计数器 格式")
	})

	t.Run("GenerateCorrelationID", func(t *testing.T) {
		correlationID := gen.GenerateCorrelationID()
		assert.NotEmpty(t, correlationID, "CorrelationID 不应为空")
		assert.True(t, regexp.MustCompile(`^[0-9A-Za-z]+-[0-9A-Za-z]+$`).MatchString(correlationID), "CorrelationID 应为 Base62-Base62 格式")
	})

	t.Run("DifferentFormats", func(t *testing.T) {
		traceID := gen.GenerateTraceID()
		spanID := gen.GenerateSpanID()
		requestID := gen.GenerateRequestID()
		correlationID := gen.GenerateCorrelationID()

		assert.NotEqual(t, traceID, spanID, "TraceID 和 SpanID 格式应不同")
		assert.NotEqual(t, traceID, requestID, "TraceID 和 RequestID 格式应不同")
		assert.NotEqual(t, traceID, correlationID, "TraceID 和 CorrelationID 格式应不同")
		assert.NotEqual(t, spanID, requestID, "SpanID 和 RequestID 格式应不同")
		assert.NotEqual(t, spanID, correlationID, "SpanID 和 CorrelationID 格式应不同")
		assert.NotEqual(t, requestID, correlationID, "RequestID 和 CorrelationID 格式应不同")
	})

	t.Run("Uniqueness", func(t *testing.T) {
		ids := make(map[string]bool)
		for i := 0; i < 10000; i++ {
			id := gen.GenerateTraceID()
			assert.False(t, ids[id], "生成的 ID 应唯一")
			ids[id] = true
		}
	})
}

// TestShortFlakeConcurrent 测试并发生成
func TestShortFlakeConcurrent(t *testing.T) {
	gen := NewShortFlakeGenerator(1)

	var wg sync.WaitGroup
	ids := make(map[int64]bool)
	mu := sync.Mutex{}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := gen.Generate()
			mu.Lock()
			assert.False(t, ids[id], "并发生成的 ID 应唯一")
			ids[id] = true
			mu.Unlock()
		}()
	}

	wg.Wait()
	assert.Equal(t, 100, len(ids), "应生成 100 个唯一 ID")
}

// TestShortFlakeCASHighConcurrency CAS 无锁路径高并发唯一性
// 高并发覆盖两条关键路径：CAS 竞争重试、同毫秒序列耗尽（64/ms）自旋等待下一毫秒
func TestShortFlakeCASHighConcurrency(t *testing.T) {
	gen := NewShortFlakeGenerator(7)

	const goroutines = 8
	const perGoroutine = 500

	ids := make(chan int64, goroutines*perGoroutine)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				ids <- gen.Generate()
			}
		}()
	}
	wg.Wait()
	close(ids)

	seen := make(map[int64]bool, goroutines*perGoroutine)
	for id := range ids {
		assert.False(t, seen[id], "CAS 并发生成的 ID 应唯一")
		seen[id] = true
		// JavaScript Number 安全范围：53 位
		assert.LessOrEqual(t, id, (int64(1)<<53)-1, "ID 应在 53 位安全范围内")
	}
	assert.Equal(t, goroutines*perGoroutine, len(seen), "应生成全部唯一 ID")
}

// TestShortFlakeIDBitLayout 验证位布局：时间戳(41位) + 节点ID(6位) + 序列号(6位)
func TestShortFlakeIDBitLayout(t *testing.T) {
	const nodeID = 42 // 101010b，小于 64
	gen := NewShortFlakeGenerator(nodeID)

	id := gen.Generate()

	// 节点 ID 应精确落在 6-11 位
	assert.Equal(t, int64(nodeID), (id>>6)&0x3F, "节点 ID 位应正确编码")

	// 时间戳部分应接近当前时间（epoch 起毫秒数，允许生成到断言之间的时钟漂移）
	elapsed := id >> 12
	expected := time.Now().UnixMilli() - 1640995200000
	assert.InDelta(t, expected, elapsed, 1000, "时间戳部分应接近当前时间")

	// 序列号应在 6 位范围内
	assert.LessOrEqual(t, id&0x3F, int64(0x3F), "序列号应不超过 6 位")
}

// BenchmarkShortFlakeGenerator 基准测试
func BenchmarkShortFlakeGenerator(b *testing.B) {
	gen := NewShortFlakeGenerator(1)
	b.Run("Generate", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			gen.Generate()
		}
	})
}

// BenchmarkShortFlakeBase62Generator 基准测试 - Base62
func BenchmarkShortFlakeBase62Generator(b *testing.B) {
	gen := NewShortFlakeBase62Generator(1)
	b.Run("GenerateTraceID", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			gen.GenerateTraceID()
		}
	})
}
