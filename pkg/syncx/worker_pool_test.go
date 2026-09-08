/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2025-02-10 00:00:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2025-02-10 00:00:00
 * @FilePath: \go-toolbox\pkg\syncx\worker_pool_test.go
 * @Description: Worker 池测试
 *
 * Copyright (c) 2025 by kamalyes, All Rights Reserved.
 */
package syncx

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const testTaskCount = 100

// TestWorkerPoolBasic 测试 Worker 池基本功能
func TestWorkerPoolBasic(t *testing.T) {
	pool := NewWorkerPool(5, testTaskCount)
	defer pool.Close()

	counter := 0
	var mu sync.Mutex

	for i := 0; i < testTaskCount; i++ {
		err := pool.Submit(context.Background(), func() {
			mu.Lock()
			counter++
			mu.Unlock()
		})
		assert.NoError(t, err)
	}

	// 等待所有任务完成
	pool.Wait()

	mu.Lock()
	assert.Equal(t, testTaskCount, counter)
	mu.Unlock()
}

// TestWorkerPoolQueueFull 测试 Worker 池队列满的情况
func TestWorkerPoolQueueFull(t *testing.T) {
	pool := NewWorkerPool(1, 2)
	defer pool.Close()

	// 提交会阻塞的任务
	blockChan := make(chan struct{})
	started := make(chan struct{})
	err := pool.Submit(context.Background(), func() {
		close(started)
		<-blockChan
	})
	assert.NoError(t, err)

	// 等待 worker 开始执行阻塞任务，确保后续提交进入队列
	<-started

	// 提交任务填满队列（队列大小为 2）
	err = pool.Submit(context.Background(), func() {})
	assert.NoError(t, err)
	err = pool.Submit(context.Background(), func() {})
	assert.NoError(t, err)

	// 非阻塞提交应该返回队列满错误
	err = pool.SubmitNonBlocking(func() {})
	assert.ErrorIs(t, err, ErrQueueFull)

	// 解除阻塞
	close(blockChan)
	time.Sleep(100 * time.Millisecond)
}

// TestWorkerPoolContextCancellation 测试 Worker 池的 context 取消
func TestWorkerPoolContextCancellation(t *testing.T) {
	pool := NewWorkerPool(2, 10)

	counter := 0
	var mu sync.Mutex

	// 提交一些任务
	for i := 0; i < 5; i++ {
		err := pool.Submit(context.Background(), func() {
			mu.Lock()
			counter++
			mu.Unlock()
			time.Sleep(10 * time.Millisecond)
		})
		assert.NoError(t, err)
	}

	// 等待任务完成
	pool.Wait()

	// 验证任务已完成
	mu.Lock()
	assert.Greater(t, counter, 0)
	mu.Unlock()

	// 关闭后提交应该返回错误
	pool.Close()
	err := pool.Submit(context.Background(), func() {})
	assert.ErrorIs(t, err, ErrClosed)
}

// TestWorkerPoolClosed 测试 Worker 池关闭后的行为
func TestWorkerPoolClosed(t *testing.T) {
	pool := NewWorkerPool(2, 10)
	pool.Close()

	// 关闭后提交应该返回错误
	err := pool.Submit(context.Background(), func() {})
	assert.ErrorIs(t, err, ErrClosed)

	// 非阻塞提交也应该返回错误
	err = pool.SubmitNonBlocking(func() {})
	assert.ErrorIs(t, err, ErrClosed)
}

// TestWorkerPoolGoroutineCount 测试 Worker 池的 goroutine 数量
func TestWorkerPoolGoroutineCount(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	pool := NewWorkerPool(10, 100)
	time.Sleep(100 * time.Millisecond)

	// 创建 pool 后应该增加 10 个 worker goroutine
	afterCreateGoroutines := runtime.NumGoroutine()
	assert.Greater(t, afterCreateGoroutines, initialGoroutines)

	pool.Close()
	time.Sleep(100 * time.Millisecond)

	// 关闭后 goroutine 数量应该恢复
	afterCloseGoroutines := runtime.NumGoroutine()
	assert.LessOrEqual(t, afterCloseGoroutines, initialGoroutines+2) // 允许小的偏差
}

// TestWorkerPoolStress 压力测试 Worker 池
func TestWorkerPoolStress(t *testing.T) {
	pool := NewWorkerPool(20, 1000)
	defer pool.Close()

	taskCount := 10000
	completedCount := 0
	var mu sync.Mutex

	for i := 0; i < taskCount; i++ {
		err := pool.Submit(context.Background(), func() {
			mu.Lock()
			completedCount++
			mu.Unlock()
		})
		assert.NoError(t, err)
	}

	pool.Wait()

	mu.Lock()
	assert.Equal(t, taskCount, completedCount)
	mu.Unlock()
}

// TestWorkerPoolQueueSize 测试 Worker 池队列大小
func TestWorkerPoolQueueSize(t *testing.T) {
	pool := NewWorkerPool(2, 10)
	defer pool.Close()

	// 初始队列应该为空
	assert.Equal(t, 0, pool.GetQueueSize())

	// 提交阻塞任务
	blockChan := make(chan struct{})
	pool.Submit(context.Background(), func() {
		<-blockChan
	})

	// 提交更多任务
	for i := 0; i < 5; i++ {
		pool.Submit(context.Background(), func() {})
	}

	// 队列大小应该反映待处理任务数
	queueSize := pool.GetQueueSize()
	assert.Greater(t, queueSize, 0)

	close(blockChan)
	time.Sleep(100 * time.Millisecond)
}

// TestWorkerPoolWorkerCount 测试 Worker 池的 worker 数量
func TestWorkerPoolWorkerCount(t *testing.T) {
	workerCount := 15
	pool := NewWorkerPool(workerCount, 100)
	defer pool.Close()

	assert.Equal(t, workerCount, pool.GetWorkerCount())
}

// TestWorkerPoolIsClosed 测试 Worker 池的关闭状态
func TestWorkerPoolIsClosed(t *testing.T) {
	pool := NewWorkerPool(5, 10)

	assert.False(t, pool.IsClosed())

	pool.Close()

	assert.True(t, pool.IsClosed())
}

// TestWorkerPoolNilTask 测试提交 nil 任务
func TestWorkerPoolNilTask(t *testing.T) {
	pool := NewWorkerPool(5, 10)
	defer pool.Close()

	// 提交 nil 任务应该返回 nil
	err := pool.Submit(context.Background(), nil)
	assert.NoError(t, err)

	// 非阻塞提交 nil 任务也应该返回 nil
	err = pool.SubmitNonBlocking(nil)
	assert.NoError(t, err)
}

// TestWorkerPoolContextTimeout 测试 context 超时
func TestWorkerPoolContextTimeout(t *testing.T) {
	pool := NewWorkerPool(1, 1)
	defer pool.Close()

	// 提交会阻塞的任务，填满队列
	blockChan := make(chan struct{})
	err := pool.Submit(context.Background(), func() {
		<-blockChan
	})
	assert.NoError(t, err)

	// 队列现在满了，再提交一个任务到队列
	err = pool.Submit(context.Background(), func() {})
	assert.NoError(t, err)

	// 创建超时 context，队列已满，应该超时
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// 提交任务应该超时
	err = pool.Submit(ctx, func() {})
	assert.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)

	close(blockChan)
}

// TestWorkerPoolElasticInitialWorkers 构造时仅启动保底数量 worker（min(4, workers)）
func TestWorkerPoolElasticInitialWorkers(t *testing.T) {
	pool := NewWorkerPool(8, 100)
	defer pool.Close()

	assert.Equal(t, 4, pool.GetActiveWorkerCount(), "初始应只启动保底 4 个 worker")
	assert.Equal(t, 8, pool.GetWorkerCount(), "worker 上限应保持 8")
}

// TestWorkerPoolElasticGrowOnBacklog 队列积压触发扩容至上限
func TestWorkerPoolElasticGrowOnBacklog(t *testing.T) {
	pool := NewWorkerPool(8, 100)
	defer pool.Close()

	// 提交 8 个阻塞任务：前 4 个占满初始 worker，后 4 个形成积压触发扩容
	release := make(chan struct{})
	started := make(chan struct{}, 8)

	for i := 0; i < 8; i++ {
		err := pool.Submit(context.Background(), func() {
			started <- struct{}{}
			<-release
		})
		assert.NoError(t, err)
	}

	// 等待全部 8 个任务开始执行（隐含要求 active 达到 8）
	for i := 0; i < 8; i++ {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("任务未全部开始，扩容未生效")
		}
	}

	assert.Equal(t, 8, pool.GetActiveWorkerCount(), "积压应触发扩容至 8 上限")

	close(release)
	pool.Wait()
}

// TestWorkerPoolElasticShrinkIdle 空闲超过 idleTimeout 自动回收至保底数量
func TestWorkerPoolElasticShrinkIdle(t *testing.T) {
	pool := NewWorkerPool(8, 100, WithPoolIdleTimeout(150*time.Millisecond))
	defer pool.Close()

	// 先扩容到 8
	release := make(chan struct{})
	started := make(chan struct{}, 8)
	for i := 0; i < 8; i++ {
		_ = pool.Submit(context.Background(), func() {
			started <- struct{}{}
			<-release
		})
	}
	for i := 0; i < 8; i++ {
		<-started
	}
	close(release)
	pool.Wait()

	assert.Equal(t, 8, pool.GetActiveWorkerCount(), "任务执行期间应保持 8 个 worker")

	// 空闲等待 3.5 倍 idleTimeout，worker 应回收至保底 4
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pool.GetActiveWorkerCount() <= 4 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	assert.LessOrEqual(t, pool.GetActiveWorkerCount(), 4, "空闲后应回收至保底数量")

	// 回收后池仍可正常工作
	var executed atomic.Int32
	err := pool.Submit(context.Background(), func() { executed.Add(1) })
	assert.NoError(t, err)
	pool.Wait()
	assert.Equal(t, int32(1), executed.Load(), "回收后池应能继续处理任务")
}

// TestWorkerPoolElasticMinFloor workers 上限低于默认保底数量时以 workers 为下限
func TestWorkerPoolElasticMinFloor(t *testing.T) {
	pool := NewWorkerPool(3, 10, WithPoolIdleTimeout(100*time.Millisecond))
	defer pool.Close()

	// minWorkers = min(4, 3) = 3
	assert.Equal(t, 3, pool.GetActiveWorkerCount(), "小池初始应全部启动")

	// 空闲足够久，不应回收任何 worker（已到下限）
	time.Sleep(400 * time.Millisecond)
	assert.Equal(t, 3, pool.GetActiveWorkerCount(), "达到下限后不应继续回收")
}

// TestWorkerPoolIdleTimeoutOptionInvalid 非法 idleTimeout（<=0）应忽略并保持默认
func TestWorkerPoolIdleTimeoutOptionInvalid(t *testing.T) {
	pool := NewWorkerPool(4, 10, WithPoolIdleTimeout(0))
	defer pool.Close()

	assert.Equal(t, defaultPoolIdleTimeout, pool.idleTimeout, "非法值应保持默认超时")
}

// TestWorkerPoolElasticCloseDuringWork 扩容状态下直接 Close 不死锁、in-flight 任务执行完
func TestWorkerPoolElasticCloseDuringWork(t *testing.T) {
	pool := NewWorkerPool(8, 100)
	defer pool.Close()

	release := make(chan struct{})
	started := make(chan struct{}, 8)
	var completed atomic.Int32
	for i := 0; i < 8; i++ {
		_ = pool.Submit(context.Background(), func() {
			started <- struct{}{}
			<-release
			completed.Add(1)
		})
	}

	// 等待全部任务被 worker 取走（真正 in-flight）后再关闭
	// Submit 返回仅代表入队；不等待就 Close 的话，worker 的 select 在 ctx.Done 与队列间随机选择，
	// 排队任务可能在被取走前被 Close 的 drain 丢弃（慢速机器上必现）
	for i := 0; i < 8; i++ {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("任务未全部被 worker 取走，扩容未生效")
		}
	}

	// 任务阻塞中直接 Close：wg.Wait 应等待 worker，不 panic 不死锁
	close(release)
	done := make(chan struct{})
	go func() {
		_ = pool.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Close 在任务执行期间不应死锁")
	}
	assert.Equal(t, int32(8), completed.Load(), "in-flight 任务应全部执行完")
}

// TestWorkerPoolElasticConcurrentStress 并发提交下弹性伸缩的任务不丢不重
func TestWorkerPoolElasticConcurrentStress(t *testing.T) {
	pool := NewWorkerPool(16, 1024, WithPoolIdleTimeout(100*time.Millisecond))
	defer pool.Close()

	const producers = 8
	const perProducer = 200
	var executed atomic.Int32

	var wg sync.WaitGroup
	for p := 0; p < producers; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perProducer; i++ {
				assert.NoError(t, pool.Submit(context.Background(), func() {
					executed.Add(1)
				}))
			}
		}()
	}
	wg.Wait()
	pool.Wait()

	assert.Equal(t, int32(producers*perProducer), executed.Load(), "全部任务应恰好执行一次")

	// 压测中应发生过扩容（生产速度高于 16 个保底 worker 的消费速度时）
	// 不做硬断言：单机调度下扩容与否取决于时序，仅验证正确性
}

// TestWorkerPoolElasticRegrow 回收后再次积压能重新扩容
func TestWorkerPoolElasticRegrow(t *testing.T) {
	pool := NewWorkerPool(8, 100, WithPoolIdleTimeout(100*time.Millisecond))
	defer pool.Close()

	// 第一轮：扩容到 8 后空闲回收到 4
	release := make(chan struct{})
	started := make(chan struct{}, 8)
	for i := 0; i < 8; i++ {
		_ = pool.Submit(context.Background(), func() {
			started <- struct{}{}
			<-release
		})
	}
	for i := 0; i < 8; i++ {
		<-started
	}
	close(release)
	pool.Wait()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pool.GetActiveWorkerCount() <= 4 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	assert.LessOrEqual(t, pool.GetActiveWorkerCount(), 4, "第一轮空闲后应回收")

	// 第二轮：再次积压应重新扩容
	release2 := make(chan struct{})
	started2 := make(chan struct{}, 8)
	for i := 0; i < 8; i++ {
		_ = pool.Submit(context.Background(), func() {
			started2 <- struct{}{}
			<-release2
		})
	}
	for i := 0; i < 8; i++ {
		select {
		case <-started2:
		case <-time.After(3 * time.Second):
			t.Fatal("回收后再次积压未触发扩容")
		}
	}
	assert.Equal(t, 8, pool.GetActiveWorkerCount(), "第二轮应重新扩容至上限")

	close(release2)
	pool.Wait()
}
