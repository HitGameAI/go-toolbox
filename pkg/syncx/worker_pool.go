/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2025-02-10 00:00:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2025-02-10 00:00:00
 * @FilePath: \go-toolbox\pkg\syncx\worker_pool.go
 * @Description: Worker 池实现，用于限制并发 goroutine 数量，防止 OOM
 *
 * Copyright (c) 2025 by kamalyes, All Rights Reserved.
 */
package syncx

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// WorkerTask 任务接口
type WorkerTask func()

// 弹性池默认参数
const (
	defaultPoolIdleTimeout = 60 * time.Second // worker 空闲多久后自动回收
	defaultPoolMinWorkers  = 4                // 空闲回收后保留的最小 worker 数
)

// PoolOption Worker 池选项函数
type PoolOption func(*WorkerPool)

// WithPoolIdleTimeout 设置空闲 worker 自动回收超时
// 不设置时使用 defaultPoolIdleTimeout（60s），仅影响回收节奏不影响并发上限
func WithPoolIdleTimeout(d time.Duration) PoolOption {
	return func(p *WorkerPool) {
		if d > 0 {
			p.idleTimeout = d
		}
	}
}

// WithWorkerPoolPanicHandler 设置 panic 恢复处理器
// 任务 panic 时调用 handler，handler 为 nil 时仅 recover 不记录（防止进程崩溃）
func WithWorkerPoolPanicHandler(handler RecoverFunc) PoolOption {
	return func(p *WorkerPool) {
		p.panicHandler = handler
	}
}

// WorkerPool 弹性 Worker 池，用于限制并发 goroutine 数量
// 防止高频操作导致 goroutine 无限增长导致 OOM
//
// 弹性策略（并发上限 workers 不变，按需伸缩）：
//   - 构造时仅启动保底数量 worker（min(4, workers)），而非全量启动
//   - Submit 发现队列有积压时按需补充 worker（不超过 workers 上限）
//   - worker 空闲超过 idleTimeout 自动退出（回收后不低于保底数量）
//
// 使用场景:
//   - PubSub 消息处理
//   - 高频任务分发
//   - 并发控制
//
// 示例:
//
//	pool := NewWorkerPool(20, 100)
//	defer pool.Close()
//
//	for i := 0; i < 1000; i++ {
//	    err := pool.Submit(ctx, func() {
//	        处理任务
//	    })
//	    if err != nil {
//	        log.Errorf("submit failed: %v", err)
//	    }
//	}
type WorkerPool struct {
	workers      int             // worker 数量上限（弹性伸缩的并发安全帽）
	queue        chan WorkerTask // 任务队列
	wg           sync.WaitGroup  // worker goroutine 等待
	taskWg       sync.WaitGroup  // 任务完成等待（提交时 +1，执行完 -1）
	panicHandler RecoverFunc     // panic 恢复处理器（nil 时仅 recover 防崩溃）
	ctx          context.Context
	cancel       context.CancelFunc
	once         sync.Once
	closed       bool
	mu           sync.Mutex
	idleTimeout  time.Duration // 空闲回收超时
	minWorkers   int           // 回收下限
	active       atomic.Int32  // 当前存活 worker 数
}

var (
	// ErrClosed 表示 Worker 池已关闭
	ErrClosed = errors.New("worker pool is closed")

	// ErrQueueFull 表示 Worker 池队列已满
	ErrQueueFull = errors.New("worker pool queue is full")
)

// NewWorkerPool 创建 Worker 池
// workers: worker 数量，建议 10-50，根据 CPU 核心数调整
// queueSize: 任务队列大小，建议 100-1000
// opts: 可选配置（如 WithWorkerPoolPanicHandler）
//
// 示例:
//
//	创建 20 个 worker，队列大小 100
//	pool := NewWorkerPool(20, 100)
//	defer pool.Close()
//
// 带 panic 恢复处理器：
//
//	pool := NewWorkerPool(20, 100, WithWorkerPoolPanicHandler(func(r interface{}) {
//	    log.Error("worker panic recovered", "panic", r)
//	}))
func NewWorkerPool(workers, queueSize int, opts ...PoolOption) *WorkerPool {
	if workers <= 0 {
		workers = 10 // 默认 10 个 worker
	}
	if queueSize <= 0 {
		queueSize = 100 // 默认队列大小 100
	}

	minWorkers := defaultPoolMinWorkers
	if minWorkers > workers {
		minWorkers = workers
	}

	ctx, cancel := context.WithCancel(context.Background())

	pool := &WorkerPool{
		workers:     workers,
		queue:       make(chan WorkerTask, queueSize),
		ctx:         ctx,
		cancel:      cancel,
		idleTimeout: defaultPoolIdleTimeout,
		minWorkers:  minWorkers,
	}
	for _, opt := range opts {
		opt(pool)
	}

	// 只启动保底数量，其余按任务积压按需补充
	for i := 0; i < minWorkers; i++ {
		pool.trySpawnWorker()
	}

	return pool
}

// trySpawnWorker 按需启动一个 worker（不超过上限则启动并返回 true）
// wg.Add 与 closed 检查同锁序完成，避免与 Close 的 wg.Wait 产生 Add/Wait 竞态
func (p *WorkerPool) trySpawnWorker() bool {
	for {
		cur := p.active.Load()
		if cur >= int32(p.workers) {
			return false
		}
		if !p.active.CompareAndSwap(cur, cur+1) {
			continue // 并发竞争，重试
		}

		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			p.active.Add(-1) // 归还名额，池已关闭不再启动
			return false
		}
		p.wg.Add(1)
		p.mu.Unlock()

		go p.worker()
		return true
	}
}

// tryRetireWorker 空闲 worker 尝试退役（低于保底数量时拒绝）
func (p *WorkerPool) tryRetireWorker() bool {
	for {
		cur := p.active.Load()
		if cur <= int32(p.minWorkers) {
			return false
		}
		if p.active.CompareAndSwap(cur, cur-1) {
			return true
		}
	}
}

// runTask 执行单个任务（panic 恢复 + taskWg 计数保持一致）
func (p *WorkerPool) runTask(task WorkerTask) {
	// defer 顺序：先 recover 再 Done，确保 panic 不影响 taskWg 计数
	defer p.taskWg.Done()
	defer RecoverWithHandler(p.panicHandler)
	if task != nil {
		task()
	}
}

// worker 工作 goroutine，从队列中取任务执行
// 任务 panic 会被 recover，确保 taskWg.Done() 始终被调用（防止 Wait/Close 死锁）
// 空闲超过 idleTimeout 且高于保底数量时自动退役
func (p *WorkerPool) worker() {
	defer p.wg.Done()

	timer := time.NewTimer(p.idleTimeout)
	defer timer.Stop()

	for {
		select {
		case <-p.ctx.Done():
			// context 已取消，优先检查，确保快速退出
			p.active.Add(-1)
			return
		case task, ok := <-p.queue:
			if !ok {
				// 队列已关闭，退出 worker
				p.active.Add(-1)
				return
			}
			// 取到任务，重置空闲计时（Go 1.23+ Reset 语义安全）
			timer.Reset(p.idleTimeout)
			p.runTask(task)
		case <-timer.C:
			// 空闲超时：退役前再捞一次队列，避免与 Submit 的竞态漏任务
			select {
			case task, ok := <-p.queue:
				if !ok {
					p.active.Add(-1)
					return
				}
				timer.Reset(p.idleTimeout)
				p.runTask(task)
			default:
				// 真空闲：高于保底数量则退役，否则继续等待
				if p.tryRetireWorker() {
					return
				}
				timer.Reset(p.idleTimeout)
			}
		}
	}
}

// Submit 提交任务到队列
// 如果队列满，会阻塞直到有空位或 context 取消
//
// 参数:
//   - ctx: 上下文，用于超时和取消控制
//   - task: 要执行的任务
//
// 返回:
//   - nil: 任务成功提交
//   - ErrClosed: Worker 池已关闭
//   - context.Err(): context 被取消或超时
//
// 示例:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//
//	err := pool.Submit(ctx, func() {
//	   处理任务
//	})
//	if err != nil {
//	    log.Errorf("submit failed: %v", err)
//	}
func (p *WorkerPool) Submit(ctx context.Context, task WorkerTask) error {
	if task == nil {
		return nil
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return ErrClosed
	}
	p.mu.Unlock()

	// 先计数再入队，避免 Wait 在「已出队但未执行」的间隙误判为完成
	p.taskWg.Add(1)
	select {
	case p.queue <- task:
		p.growOnBacklog() // 队列有积压时按需补充 worker
		return nil
	case <-ctx.Done():
		p.taskWg.Done()
		return ctx.Err()
	case <-p.ctx.Done():
		p.taskWg.Done()
		return ErrClosed
	}
}

// growOnBacklog 队列存在积压时补充 worker（CAS 上限保护，满编时为一次原子读）
func (p *WorkerPool) growOnBacklog() {
	if len(p.queue) > 0 {
		p.trySpawnWorker()
	}
}

// SubmitNonBlocking 非阻塞提交任务
// 如果队列满，返回错误而不是阻塞
//
// 参数:
//   - task: 要执行的任务
//
// 返回:
//   - nil: 任务成功提交
//   - ErrClosed: Worker 池已关闭
//   - ErrQueueFull: 队列已满
//
// 示例:
//
//	err := pool.SubmitNonBlocking(func() {
//	   处理任务
//	})
//	if err == ErrQueueFull {
//	    log.Warn("queue is full, task dropped")
//	}
func (p *WorkerPool) SubmitNonBlocking(task WorkerTask) error {
	if task == nil {
		return nil
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return ErrClosed
	}
	p.mu.Unlock()

	p.taskWg.Add(1)
	select {
	case p.queue <- task:
		p.growOnBacklog() // 队列有积压时按需补充 worker
		return nil
	default:
		p.taskWg.Done()
		return ErrQueueFull
	}
}

// Wait 等待所有已提交的任务完成
// 不关闭 pool，可以继续提交新任务
//
// 示例:
//
//	pool := NewWorkerPool(20, 100)
//	提交任务 ...
//	pool.Wait()  // 等待所有任务完成
//	继续提交任务 ...
func (p *WorkerPool) Wait() {
	p.taskWg.Wait()
}

// Close 关闭 Worker 池，等待所有任务完成
// 调用此方法后，所有新的提交都会返回 ErrClosed
//
// 注意：不关闭 queue channel，避免 Submit 在检查 closed 标志后
// 仍向已关闭 channel 发送数据导致 panic。worker 通过 ctx.Done() 退出
//
// 示例:
//
//	pool := NewWorkerPool(20, 100)
//	提交任务 ...
//	pool.Close()  // 等待所有任务完成后返回
func (p *WorkerPool) Close() error {
	p.once.Do(func() {
		p.mu.Lock()
		p.closed = true
		p.mu.Unlock()

		// 取消 context，通知所有 worker 退出
		// 不关闭 queue channel，避免 Submit panic
		p.cancel()

		// 等待所有 worker 完成（in-flight 任务也会执行完）
		p.wg.Wait()

		// 排空队列中未被 worker 取走的任务，保持 taskWg 计数一致
		for drain := true; drain; {
			select {
			case <-p.queue:
				p.taskWg.Done()
			default:
				drain = false
			}
		}
	})

	return nil
}

// GetQueueSize 获取队列中待处理任务数
func (p *WorkerPool) GetQueueSize() int {
	return len(p.queue)
}

// GetWorkerCount 获取 worker 数量上限（弹性伸缩的并发安全帽）
func (p *WorkerPool) GetWorkerCount() int {
	return p.workers
}

// GetActiveWorkerCount 获取当前存活的 worker 数量
func (p *WorkerPool) GetActiveWorkerCount() int {
	return int(p.active.Load())
}

// IsClosed 检查 Worker 池是否已关闭
func (p *WorkerPool) IsClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}
