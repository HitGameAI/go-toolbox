/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-08-20 09:00:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-08-20 09:00:00
 * @FilePath: \go-toolbox\pkg\syncx\timewheel_test.go
 * @Description: 分片时间轮测试
 *   - 基础调度/取消/刷新/停止
 *   - 并发安全（-race）
 *   - 心跳场景集成模拟
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package syncx

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// newTestTimer 创建测试用时间轮（短 tick + 少 shard，加速测试）
func newTestTimer() *HashedWheelTimer {
	return NewHashedWheelTimer(
		WithTimerShardCount(4),
		WithTimerBucketCount(16),
		WithTimerTickInterval(10*time.Millisecond),
	)
}

// waitFired 轮询等待 fired 达到 target 或超时
func waitFired(t *testing.T, fired *atomic.Int32, target int32, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fired.Load() >= target {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("等待触发超时：期望 %d，实际 %d", target, fired.Load())
}

// ============================================================================
// 基础功能测试
// ============================================================================

// TestTimerBasicSchedule 测试基础调度：任务在延迟后触发
func TestTimerBasicSchedule(t *testing.T) {
	timer := newTestTimer()
	defer timer.Stop()

	var fired atomic.Int32
	timer.Schedule(50*time.Millisecond, func() {
		fired.Add(1)
	})

	waitFired(t, &fired, 1, 200*time.Millisecond)
	assert.Equal(t, int32(1), fired.Load(), "任务应该触发一次")
}

// TestTimerScheduleNoEarlyFire 测试任务不会提前触发
func TestTimerScheduleNoEarlyFire(t *testing.T) {
	timer := newTestTimer()
	defer timer.Stop()

	var fired atomic.Int32
	timer.Schedule(100*time.Millisecond, func() {
		fired.Add(1)
	})

	// 任务 100ms 后才触发；用 assert.Never 在 80ms 窗口内持续轮询断言 fired 保持 0，
	// 替代 sleep+assert（sleep 在 -race 负载下会过睡越过触发点导致 flaky）
	assert.Never(t, func() bool {
		return fired.Load() > 0
	}, 80*time.Millisecond, 5*time.Millisecond, "任务不应在延迟前触发")

	waitFired(t, &fired, 1, 200*time.Millisecond)
	assert.Equal(t, int32(1), fired.Load(), "任务应该在延迟后触发")
}

// TestTimerScheduleMultiple 测试多个任务并发调度
func TestTimerScheduleMultiple(t *testing.T) {
	timer := newTestTimer()
	defer timer.Stop()

	var fired atomic.Int32
	for i := 0; i < 100; i++ {
		timer.Schedule(50*time.Millisecond, func() {
			fired.Add(1)
		})
	}

	waitFired(t, &fired, 100, 300*time.Millisecond)
	assert.Equal(t, int32(100), fired.Load(), "100 个任务应全部触发")
}

// TestTimerScheduleWithKey 测试带 key 调度：同 key 覆盖取消旧任务
func TestTimerScheduleWithKey(t *testing.T) {
	timer := newTestTimer()
	defer timer.Stop()

	var fired1, fired2 atomic.Int32

	// 第一次调度 key="client-1"
	timer.ScheduleWithKey("client-1", 50*time.Millisecond, func() {
		fired1.Add(1)
	})

	// 覆盖同 key，旧任务应被惰性取消
	timer.ScheduleWithKey("client-1", 100*time.Millisecond, func() {
		fired2.Add(1)
	})

	waitFired(t, &fired2, 1, 250*time.Millisecond)
	assert.Equal(t, int32(0), fired1.Load(), "旧任务应被取消，不触发")
	assert.Equal(t, int32(1), fired2.Load(), "新任务应触发")
}

// TestTimerCancel 测试取消指定任务
func TestTimerCancel(t *testing.T) {
	timer := newTestTimer()
	defer timer.Stop()

	var fired atomic.Int32
	task := timer.Schedule(80*time.Millisecond, func() {
		fired.Add(1)
	})

	// 取消任务
	timer.Cancel(task)

	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, int32(0), fired.Load(), "已取消的任务不应触发")
}

// TestTimerCancelByKey 测试按 key 取消
func TestTimerCancelByKey(t *testing.T) {
	timer := newTestTimer()
	defer timer.Stop()

	var fired atomic.Int32
	timer.ScheduleWithKey("client-2", 80*time.Millisecond, func() {
		fired.Add(1)
	})

	timer.CancelByKey("client-2")

	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, int32(0), fired.Load(), "按 key 取消的任务不应触发")
}

// TestTimerCancelByKeyNotFound 取消不存在的 key 不 panic
func TestTimerCancelByKeyNotFound(t *testing.T) {
	timer := newTestTimer()
	defer timer.Stop()

	assert.NotPanics(t, func() {
		timer.CancelByKey("non-existent-key")
	})
}

// TestTimerCancelNil 取消 nil 任务不 panic
func TestTimerCancelNil(t *testing.T) {
	timer := newTestTimer()
	defer timer.Stop()

	assert.NotPanics(t, func() {
		timer.Cancel(nil)
	})
}

// TestTimerRefresh 测试刷新：取消旧 + 调度新
func TestTimerRefresh(t *testing.T) {
	timer := newTestTimer()
	defer timer.Stop()

	var fired atomic.Int32

	// 初始调度
	timer.ScheduleWithKey("client-3", 50*time.Millisecond, func() {
		fired.Add(1)
	})

	// 在到期前刷新（延长延迟）
	time.Sleep(20 * time.Millisecond)
	timer.Refresh("client-3", 80*time.Millisecond, func() {
		fired.Add(1)
	})

	waitFired(t, &fired, 1, 300*time.Millisecond)
	assert.Equal(t, int32(1), fired.Load(), "刷新后只有新任务触发一次")
}

// TestTimerRefreshRepeated 测试连续刷新（心跳场景核心）
func TestTimerRefreshRepeated(t *testing.T) {
	timer := newTestTimer()
	defer timer.Stop()

	var fired atomic.Int32

	// 模拟心跳：每 30ms 刷新一次，持续 200ms
	timer.ScheduleWithKey("heartbeat-client", 60*time.Millisecond, func() {
		fired.Add(1)
	})

	for i := 0; i < 6; i++ {
		time.Sleep(30 * time.Millisecond)
		timer.Refresh("heartbeat-client", 60*time.Millisecond, func() {
			fired.Add(1)
		})
	}

	// 持续刷新期间不应触发
	assert.Equal(t, int32(0), fired.Load(), "持续刷新期间任务不应触发")

	// 停止刷新后应触发
	waitFired(t, &fired, 1, 200*time.Millisecond)
	assert.Equal(t, int32(1), fired.Load(), "停止刷新后任务应触发一次")
}

// TestTimerLargeRounds 测试大延迟（多圈绕行）
func TestTimerLargeRounds(t *testing.T) {
	// bucketCount=16, tickInterval=10ms → 一圈 160ms
	// 延迟 500ms → 约 50 ticks → 3 圈
	timer := newTestTimer()
	defer timer.Stop()

	var fired atomic.Int32
	timer.Schedule(500*time.Millisecond, func() {
		fired.Add(1)
	})

	// 任务 500ms 后才触发；用 assert.Never 在 400ms 窗口内持续断言 fired 保持 0
	assert.Never(t, func() bool {
		return fired.Load() > 0
	}, 400*time.Millisecond, 10*time.Millisecond, "大延迟任务不应提前触发")

	// Windows 定时器分辨率 15.6ms：50 ticks 实际 ≈ 780ms > 名义 500ms，
	// 原 300ms 余量（截止 ~700ms）会产生平台性假失败；本测试只验证多圈任务最终触发
	// （圈数丢失类 bug 会导致永不触发），不验证精度，放宽到 1s 留足负载余量
	waitFired(t, &fired, 1, 1*time.Second)
	assert.Equal(t, int32(1), fired.Load(), "大延迟任务应最终触发")
}

// TestTimerStop 测试停止后不再触发
func TestTimerStop(t *testing.T) {
	timer := newTestTimer()

	var fired atomic.Int32
	timer.Schedule(100*time.Millisecond, func() {
		fired.Add(1)
	})

	// 立即停止
	timer.Stop()

	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, int32(0), fired.Load(), "停止后任务不应触发")
}

// TestTimerStopIdempotent 测试重复停止不 panic
func TestTimerStopIdempotent(t *testing.T) {
	timer := newTestTimer()

	assert.NotPanics(t, func() {
		timer.Stop()
		timer.Stop()
	})
}

// TestTimerScheduleAfterStop 测试停止后调度返回 nil
func TestTimerScheduleAfterStop(t *testing.T) {
	timer := newTestTimer()
	timer.Stop()

	result := timer.Schedule(50*time.Millisecond, func() {})
	assert.Nil(t, result, "停止后调度应返回 nil")
}

// TestTimerScheduleInvalid 测试无效参数调度
func TestTimerScheduleInvalid(t *testing.T) {
	timer := newTestTimer()
	defer timer.Stop()

	assert.Nil(t, timer.Schedule(0, func() {}), "零延迟应返回 nil")
	assert.Nil(t, timer.Schedule(-1, func() {}), "负延迟应返回 nil")
	assert.Nil(t, timer.Schedule(50*time.Millisecond, nil), "nil 回调应返回 nil")
}

// ============================================================================
// 统计测试
// ============================================================================

// TestTimerStats 测试统计信息
// 注意：惰性取消不立即更新 ActiveTasks，需等 worker 处理对应 bucket 后才递减
func TestTimerStats(t *testing.T) {
	timer := newTestTimer()
	defer timer.Stop()

	var fired atomic.Int32
	// 调度 3 个任务，取消 1 个，2 个会触发
	t1 := timer.Schedule(50*time.Millisecond, func() { fired.Add(1) })
	timer.Schedule(50*time.Millisecond, func() { fired.Add(1) })
	timer.Schedule(50*time.Millisecond, func() { fired.Add(1) })

	// 取消一个（惰性取消，ActiveTasks 暂不递减）
	timer.Cancel(t1)

	// 等待所有任务处理完毕（触发 + 清理取消）
	waitFired(t, &fired, 2, 200*time.Millisecond)

	// 再等一拍确保 worker 清理完取消任务
	time.Sleep(20 * time.Millisecond)

	stats := timer.Stats()
	assert.Equal(t, int64(0), stats.ActiveTasks, "所有任务处理完毕后应无活跃任务")
	assert.Equal(t, int64(2), stats.CompletedTasks, "应有 2 个已完成任务")
	assert.Equal(t, int64(1), stats.CancelledTasks, "应有 1 个已取消任务")
}

// ============================================================================
// 并发安全测试（-race）
// ============================================================================

// TestTimerConcurrentScheduleCancel 并发调度+取消（race 检测）
func TestTimerConcurrentScheduleCancel(t *testing.T) {
	timer := newTestTimer()
	defer timer.Stop()

	var fired atomic.Int32
	var wg sync.WaitGroup

	// 50 个 goroutine 并发调度和取消
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("client-%d", id)
			for j := 0; j < 20; j++ {
				task := timer.ScheduleWithKey(key, 30*time.Millisecond, func() {
					fired.Add(1)
				})
				if j%2 == 0 && task != nil {
					timer.Cancel(task)
				}
			}
		}(i)
	}

	wg.Wait()

	// 等待所有剩余任务触发
	time.Sleep(200 * time.Millisecond)

	stats := timer.Stats()
	t.Logf("并发测试结果: fired=%d, completed=%d, cancelled=%d, active=%d",
		fired.Load(), stats.CompletedTasks, stats.CancelledTasks, stats.ActiveTasks)
	assert.Equal(t, int64(0), stats.ActiveTasks, "所有任务应已处理完毕")
}

// TestTimerConcurrentRefreshSameKey 多 goroutine 竞争刷新同一 key
func TestTimerConcurrentRefreshSameKey(t *testing.T) {
	timer := newTestTimer()
	defer timer.Stop()

	var fired atomic.Int32
	var wg sync.WaitGroup

	// 10 个 goroutine 不断刷新同一 key（模拟多线程收到同一客户端心跳）
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				timer.Refresh("shared-key", 50*time.Millisecond, func() {
					fired.Add(1)
				})
			}
		}()
	}

	wg.Wait()

	// 等待最终触发
	waitFired(t, &fired, 1, 300*time.Millisecond)

	// 同一 key 最终只应触发一次（最后一个刷新的生效）
	assert.Equal(t, int32(1), fired.Load(), "同一 key 应只触发一次")
}

// TestTimerConcurrentMixedOps 混合并发操作（schedule/cancel/refresh）
func TestTimerConcurrentMixedOps(t *testing.T) {
	timer := NewHashedWheelTimer(
		WithTimerShardCount(8),
		WithTimerBucketCount(32),
		WithTimerTickInterval(5*time.Millisecond),
	)
	defer timer.Stop()

	var fired atomic.Int32
	var wg sync.WaitGroup

	// 并发调度无 key 任务
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				timer.Schedule(20*time.Millisecond, func() { fired.Add(1) })
			}
		}()
	}

	// 并发 keyed 调度 + 刷新
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("k-%d", id)
			for j := 0; j < 50; j++ {
				timer.ScheduleWithKey(key, 30*time.Millisecond, func() { fired.Add(1) })
			}
		}(i)
	}

	wg.Wait()

	// 等待所有任务处理
	time.Sleep(200 * time.Millisecond)

	stats := timer.Stats()
	assert.Equal(t, int64(0), stats.ActiveTasks, "所有任务应已处理")
	t.Logf("混合并发: fired=%d, completed=%d, cancelled=%d",
		fired.Load(), stats.CompletedTasks, stats.CancelledTasks)
}

// ============================================================================
// 心跳场景集成模拟
// ============================================================================

// TestTimerHeartbeatScenario 模拟真实心跳场景
// 场景：注册 → PING刷新 → 正常断开(取消) / 超时断开(触发)
func TestTimerHeartbeatScenario(t *testing.T) {
	timer := newTestTimer()
	defer timer.Stop()

	var timeoutFired atomic.Int32

	// 场景1：客户端持续心跳，不超时
	client1Key := "conn:client-1"
	timer.ScheduleWithKey(client1Key, 100*time.Millisecond, func() {
		timeoutFired.Add(1)
	})

	// 模拟 3 次心跳刷新
	for i := 0; i < 3; i++ {
		time.Sleep(40 * time.Millisecond)
		timer.Refresh(client1Key, 100*time.Millisecond, func() {
			timeoutFired.Add(1)
		})
	}
	// 刷新期间不应触发
	assert.Equal(t, int32(0), timeoutFired.Load(), "场景1: 刷新期间不应超时")

	// 客户端1正常断开 → 主动取消
	timer.CancelByKey(client1Key)
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(0), timeoutFired.Load(), "场景1: 主动取消后不应超时")

	// 场景2：客户端无心跳 → 超时触发
	client2Key := "conn:client-2"
	timer.ScheduleWithKey(client2Key, 80*time.Millisecond, func() {
		timeoutFired.Add(1)
	})

	waitFired(t, &timeoutFired, 1, 250*time.Millisecond)
	assert.Equal(t, int32(1), timeoutFired.Load(), "场景2: 无心跳应超时触发")
}

// TestTimerExecutorPanicRecovery 测试回调 panic 不影响 worker
func TestTimerExecutorPanicRecovery(t *testing.T) {
	timer := newTestTimer()
	defer timer.Stop()

	var fired atomic.Int32

	// 第一个任务 panic
	timer.Schedule(30*time.Millisecond, func() {
		fired.Add(1)
		panic("test panic")
	})

	// 第二个任务正常
	timer.Schedule(50*time.Millisecond, func() {
		fired.Add(1)
	})

	// 等待两个任务都处理完
	waitFired(t, &fired, 2, 200*time.Millisecond)
	assert.Equal(t, int32(2), fired.Load(), "panic 不应影响后续任务执行")
}

// TestTimerCustomExecutor 测试自定义执行器
func TestTimerCustomExecutor(t *testing.T) {
	var executedOnCaller atomic.Int32
	executor := func(f func()) {
		executedOnCaller.Add(1)
		f() // 同步执行
	}

	timer := NewHashedWheelTimer(
		WithTimerShardCount(2),
		WithTimerBucketCount(8),
		WithTimerTickInterval(10*time.Millisecond),
		WithTimerExecutor(executor),
	)
	defer timer.Stop()

	var fired atomic.Int32
	timer.Schedule(30*time.Millisecond, func() { fired.Add(1) })

	waitFired(t, &fired, 1, 200*time.Millisecond)
	assert.Equal(t, int32(1), fired.Load(), "任务应通过自定义执行器触发")
	assert.Equal(t, int32(1), executedOnCaller.Load(), "自定义执行器应被调用")
}

// ============================================================================
// 精度回归测试（消除"延迟整圈"竞态后锁住回归）
// newTestTimer: 4 shard/16 bucket/10ms tick → 1 圈 = 160ms
// 重构前 schedule 读 currentPos 与 worker 推进存在竞态，短延迟任务最坏延迟整圈（160ms）
// 重构后 shard.mu 同步 currentTick 读写，触发精度严格 ±1 tick
// ============================================================================

// TestTimerShortDelayPrecision 验证短延迟任务不延迟整圈触发
// delay=5ms → ticks=1，重构前竞态触发时任务会延迟到 ~160ms（1 圈）后才触发
// 重构后应在 1~2 tick（10~20ms）内触发，断言 100ms 内触发（远小于 1 圈 160ms，
// 留足 CI 负载抖动余量）
func TestTimerShortDelayPrecision(t *testing.T) {
	timer := newTestTimer()
	defer timer.Stop()

	var fired atomic.Int32
	start := time.Now()
	timer.Schedule(5*time.Millisecond, func() {
		fired.Add(1)
	})

	waitFired(t, &fired, 1, 100*time.Millisecond)
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 100*time.Millisecond, "短延迟任务不应延迟整圈（160ms）触发，实际 %v", elapsed)
}

// TestTimerConcurrentShortDelayPrecision 并发调度短延迟任务，全部应快速触发
// 并发提高 schedule/worker 竞态触发概率，重构前会有部分任务延迟到 1 圈后
// 阈值 100ms：回归目标是抓住"延迟整圈（160ms）"竞态，100ms 仍能可靠拦截；
// 不用 50ms 是因为 100 个 goroutine 创建 + 调度在 CI 慢机器上本身就要几十 ms
// （实测 macOS runner 53ms 通过），卡太紧只会产生负载相关的假失败
func TestTimerConcurrentShortDelayPrecision(t *testing.T) {
	timer := newTestTimer()
	defer timer.Stop()

	const n = 100
	var fired atomic.Int32
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			timer.Schedule(5*time.Millisecond, func() {
				fired.Add(1)
			})
		}()
	}
	wg.Wait()

	waitFired(t, &fired, int32(n), 100*time.Millisecond)
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 100*time.Millisecond, "并发短延迟任务应全部在 1 圈内触发，实际 %v", elapsed)
	assert.Equal(t, int32(n), fired.Load(), "全部任务应触发")
}

// TestTimerBoundaryTicks 验证边界 tick 数的触发精度
//   - delay=tickInterval（10ms）：ticks=1，应在 [1,2] tick（10~20ms）内触发
//   - delay=bucketCount*tickInterval（160ms）：ticks=16，rounds=1，应在 [16,17] tick（160~170ms）内触发
//
// 阈值取圈边界安全值而非理论 tick 上界：Windows CI 定时器分辨率 15.6ms + 负载抖动，
// 实测 1 tick 任务 33ms、16 tick 任务 243ms——卡理论值（30ms/200ms）只会产生平台性假失败。
// 回归目标是圈数正确：1 tick 任务 < 160ms（1 圈，错圈会延迟到 ~160ms+），
// rounds=1 任务 < 320ms（2 圈，圈计数错误会延迟到 ~320ms+）
func TestTimerBoundaryTicks(t *testing.T) {
	timer := newTestTimer() // tick=10ms, bucketCount=16
	defer timer.Stop()

	// 边界1：delay = 1 tick
	var fired1 atomic.Int32
	start1 := time.Now()
	timer.Schedule(10*time.Millisecond, func() { fired1.Add(1) })
	waitFired(t, &fired1, 1, 150*time.Millisecond)
	elapsed1 := time.Since(start1)
	assert.Less(t, elapsed1, 100*time.Millisecond, "1 tick 任务应在 1 圈内触发，实际 %v", elapsed1)

	// 边界2：delay = bucketCount tick（rounds=1 路径）
	var fired2 atomic.Int32
	start2 := time.Now()
	timer.Schedule(160*time.Millisecond, func() { fired2.Add(1) })
	waitFired(t, &fired2, 1, 310*time.Millisecond)
	elapsed2 := time.Since(start2)
	// rounds=1：worker 第 1 次扫到 bucket 减圈，第 2 次触发，正常应在 160~180ms；
	// 圈计数错误才会延迟到 2 圈（320ms+），断言 300ms 已能可靠拦截
	assert.Less(t, elapsed2, 300*time.Millisecond, "16 tick（rounds=1）任务应在 2 圈内触发，实际 %v", elapsed2)
}

// TestHashedWheelTimerDefaults 验证无参构造的库默认值
// 16 分片 × 10ms tick：秒级超时语义足够，空转开销可忽略（对齐 go-config wsc.TimerConfig 默认值）
func TestHashedWheelTimerDefaults(t *testing.T) {
	timer := NewHashedWheelTimer()
	defer timer.Stop()

	assert.Equal(t, defaultTimerShardCount, timer.shardCount, "默认分片数应为 16")
	assert.Len(t, timer.shards, defaultTimerShardCount, "应创建 16 个 shard goroutine")
	for i, shard := range timer.shards {
		assert.Equal(t, defaultTimerTickInterval, shard.tickInterval, "shard[%d] tick 间隔应为 10ms", i)
	}
}

// TestHashedWheelTimerDefaultTickFire 默认 10ms tick 下任务正常触发（不提前、误差 ±1 tick 内）
func TestHashedWheelTimerDefaultTickFire(t *testing.T) {
	timer := NewHashedWheelTimer()
	defer timer.Stop()

	done := make(chan time.Time, 1)
	timer.Schedule(100*time.Millisecond, func() {
		done <- time.Now()
	})

	start := time.Now()
	select {
	case fired := <-done:
		elapsed := fired.Sub(start)
		// 向上取整保证不提前触发；100ms 延迟 + 10ms tick 下最多晚一个 tick + 调度余量
		assert.GreaterOrEqual(t, elapsed, 100*time.Millisecond, "任务不应提前触发")
		assert.LessOrEqual(t, elapsed, 200*time.Millisecond, "任务触发延迟应在一个 tick + 调度余量内")
	case <-time.After(3 * time.Second):
		t.Fatal("默认 tick 配置下任务应在 3s 内触发")
	}
}

// TestHashedWheelTimerDefaultStats 默认配置下统计信息准确
func TestHashedWheelTimerDefaultStats(t *testing.T) {
	timer := NewHashedWheelTimer()
	defer timer.Stop()

	const taskCount = 100
	var completed atomic.Int64
	done := make(chan struct{})
	for i := 0; i < taskCount; i++ {
		timer.Schedule(20*time.Millisecond, func() {
			completed.Add(1)
		})
	}

	go func() {
		for completed.Load() < taskCount {
			time.Sleep(10 * time.Millisecond)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("任务未全部完成: %d/%d", completed.Load(), taskCount)
	}

	// 等统计计数落地
	time.Sleep(50 * time.Millisecond)
	stats := timer.Stats()
	assert.Equal(t, int64(taskCount), stats.CompletedTasks, "完成任务数应准确")
	assert.Equal(t, int64(0), stats.ActiveTasks, "全部完成后活跃任务应为 0")
}
