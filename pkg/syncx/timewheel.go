/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-08-20 09:00:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-08-20 09:00:00
 * @FilePath: \go-toolbox\pkg\syncx\timewheel.go
 * @Description:
 * 分片时间轮（HashedWheelTimer）——16 分片 + 双向链表 bucket + 惰性取消，
 * O(1) 调度/取消/刷新，替代 O(N) 全量扫描，适用于百万级连接心跳超时管理
 *
 * Copyright (c) 2025 by kamalyes, All Rights Reserved.
 */
package syncx

import (
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// 分片时间轮（Hashed Wheel Timer）
// ============================================================================
//
// 设计要点：
//   - 16 个 shard 分散锁竞争，每个 shard 独立 wheel + worker goroutine
//   - 每个 wheel 是环形 bucket 数组（默认 512），每个 bucket 持有任务双向链表
//   - FNV-1a hash 分配 shard，与 sharded_registry 模式对齐
//   - 惰性取消：atomic.Bool 标记，worker 遍历时统一清理（取消 O(1) 无锁开销）
//   - O(1) 调度 / O(1) 取消 / O(1) 心跳刷新（取消旧 + 调度新）
//   - 零额外分配：任务复用双向链表节点，无中间 slice
//
// 精度说明：
//   - 触发精度严格为 ±1 tick（默认 1ms），不保证精确到纳秒
//   - 通过 shard.mu 同步 currentTick 读写，消除 schedule 与 worker 的竞态，
//     任务不会因调度竞态而延迟整圈触发
//   - 任务永远不会提前触发（ticks 向上取整 + rounds 保证）
//
// 适用场景：
//   - 百万级连接心跳超时管理（替代 O(N) 全量扫描）
//   - 跨节点投递 ACK 超时兜底
//   - 任意大量短/中周期定时任务管理

const (
	defaultTimerShardCount   = 16                    // 默认分片数（秒级超时场景足够；十万级以上活跃任务时显式调大）
	defaultTimerBucketCount  = 512                   // 默认每分片 bucket 数
	defaultTimerTickInterval = 10 * time.Millisecond // 默认 tick 间隔（10ms：16 分片共 1600 tick/s，空转开销可忽略；亚秒级精度需求时显式调小）
)

// Timer 时间轮定时器接口
type Timer interface {
	// Schedule 调度一个无 key 的定时任务（轮询分配 shard）
	Schedule(delay time.Duration, task func()) *TimerTask
	// ScheduleWithKey 调度一个带 key 的定时任务（同 key 覆盖会惰性取消旧任务）
	ScheduleWithKey(key string, delay time.Duration, task func()) *TimerTask
	// Refresh 刷新任务（O(1) 心跳刷新：取消旧 + 调度新，等价于 ScheduleWithKey）
	Refresh(key string, delay time.Duration, task func()) *TimerTask
	// Cancel 取消指定任务（惰性取消，O(1)）
	Cancel(t *TimerTask)
	// CancelByKey 按 key 取消任务（惰性取消，O(1)）
	CancelByKey(key string)
	// Stop 停止时间轮（停止所有 worker，不再执行待触发任务）
	Stop()
	// Stats 返回时间轮统计信息
	Stats() TimerStats
}

// TimerTask 时间轮任务（双向链表节点）
type TimerTask struct {
	key       string      // 任务键（空表示无 key 任务）
	callback  func()      // 回调函数
	rounds    int64       // 剩余圈数（>0 表示需绕整圈后才到期）
	bucketIdx int         // 所在 bucket 索引
	shardIdx  int         // 所在 shard 索引
	cancelled atomic.Bool // 惰性取消标记
	prev      *TimerTask  // 前驱节点
	next      *TimerTask  // 后继节点
	wheel     *HashedWheelTimer
}

// TimerStats 时间轮统计信息
type TimerStats struct {
	ActiveTasks    int64 // 活跃任务数
	CompletedTasks int64 // 已完成任务数（已执行回调）
	CancelledTasks int64 // 已取消任务数（惰性取消）
}

// HashedWheelTimer 分片时间轮主实现
type HashedWheelTimer struct {
	shards     []*wheelShard
	shardCount int                             // 分片数
	mask       int                             // shardCount-1，位运算取模
	keyIndex   *ShardedMap[string, *TimerTask] // key → *TimerTask（仅 keyed 任务，类型化避免 sync.Map 接口装箱）
	rrCounter  atomic.Int64                    // 无 key 任务的轮询分配计数器
	started    atomic.Bool
	stopped    atomic.Bool
	executor   func(func()) // 回调执行器（默认安全异步执行）
}

// wheelShard 单个分片时间轮
//
// 性能设计：
//   - completed/cancelled 计数器下沉到 shard，避免 worker 路径对 parent 全局原子
//     的跨 shard cache-line 弹跳（百万级 Refresh 下每秒百万次原子加）
type wheelShard struct {
	mu             sync.Mutex   // 保护 currentTick 读写 + 所有 bucket 的 head
	buckets        []*TimerTask // 每个 bucket 直接持有链表头指针（无包装结构，少一次间接寻址）
	bucketCount    int
	mask           int // bucketCount-1，位运算取模
	tickInterval   time.Duration
	currentTick    int // worker 下一个待处理 bucket 位置（由 mu 保护，消除 schedule/worker 竞态）
	stopCh         chan struct{}
	wg             sync.WaitGroup
	parent         *HashedWheelTimer
	taskCount      atomic.Int64 // 本 shard 活跃任务数（schedule/cancel 写）
	completedCount atomic.Int64 // 本 shard 已完成任务数（worker 写）
	cancelledCount atomic.Int64 // 本 shard 已取消任务数（worker 写）
}

// timerConfig 时间轮初始化配置（私有，避免外部直接修改）
type timerConfig struct {
	shardCount   int
	bucketCount  int
	tickInterval time.Duration
	executor     func(func())
}

// TimerOption 时间轮配置选项
type TimerOption func(*timerConfig)

// WithTimerShardCount 设置分片数量（必须是 2 的幂）
func WithTimerShardCount(count int) TimerOption {
	return func(c *timerConfig) {
		if count > 0 && count&(count-1) == 0 {
			c.shardCount = count
		}
	}
}

// WithTimerBucketCount 设置每分片 bucket 数量（必须是 2 的幂）
func WithTimerBucketCount(count int) TimerOption {
	return func(c *timerConfig) {
		if count > 0 && count&(count-1) == 0 {
			c.bucketCount = count
		}
	}
}

// WithTimerTickInterval 设置 tick 间隔
func WithTimerTickInterval(d time.Duration) TimerOption {
	return func(c *timerConfig) {
		if d > 0 {
			c.tickInterval = d
		}
	}
}

// WithTimerExecutor 设置回调执行器
func WithTimerExecutor(fn func(func())) TimerOption {
	return func(c *timerConfig) {
		if fn != nil {
			c.executor = fn
		}
	}
}

// NewHashedWheelTimer 创建分片时间轮
func NewHashedWheelTimer(opts ...TimerOption) *HashedWheelTimer {
	cfg := &timerConfig{
		shardCount:   defaultTimerShardCount,
		bucketCount:  defaultTimerBucketCount,
		tickInterval: defaultTimerTickInterval,
		executor:     safeGoExec,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	t := &HashedWheelTimer{
		shardCount: cfg.shardCount,
		mask:       cfg.shardCount - 1,
		executor:   cfg.executor,
		// keyIndex 与时间轮同分片数（FNV-1a hash），类型化 map 避免 sync.Map 接口装箱
		keyIndex: NewShardedMap[string, *TimerTask](cfg.shardCount),
	}

	t.shards = make([]*wheelShard, cfg.shardCount)
	for i := range t.shards {
		shard := &wheelShard{
			buckets:      make([]*TimerTask, cfg.bucketCount), // nil 即空 bucket，无需逐个分配
			bucketCount:  cfg.bucketCount,
			mask:         cfg.bucketCount - 1,
			tickInterval: cfg.tickInterval,
			stopCh:       make(chan struct{}),
			parent:       t,
		}
		t.shards[i] = shard
	}

	t.started.Store(true)
	for _, shard := range t.shards {
		shard.wg.Add(1)
		go shard.run()
	}

	return t
}

// safeGoExec 默认回调执行器：异步执行 + panic 恢复
// 防止单个任务 panic 导致 worker goroutine 崩溃
func safeGoExec(f func()) {
	go func() {
		defer func() { _ = recover() }()
		f()
	}()
}

// run shard worker 主循环：每 tick 推进一个 bucket
func (s *wheelShard) run() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.processCurrentBucket()
		}
	}
}

// processCurrentBucket 处理当前 bucket 的所有任务
// 三阶段：持 shard.mu 提取链表并推进 currentTick → 锁外分类处理 → 持 shard.mu 挂回未到期任务
// currentTick 推进与 schedule 的读取互斥，消除"任务落入已扫过 bucket"的竞态
func (s *wheelShard) processCurrentBucket() {
	// Phase 1：持锁提取 bucket 链表并推进 currentTick
	s.mu.Lock()
	pos := s.currentTick
	head := s.buckets[pos]
	s.buckets[pos] = nil
	s.currentTick = (pos + 1) & s.mask
	s.mu.Unlock()

	if head == nil {
		return
	}

	// 提取 parent 到局部变量，避免热循环中重复解引用
	parent := s.parent
	keyIndex := parent.keyIndex

	// Phase 2：遍历提取的任务，分类处理（锁外执行，避免回调阻塞 worker）
	var keepHead, keepTail *TimerTask
	current := head
	for current != nil {
		next := current.next
		current.prev = nil
		current.next = nil

		if current.cancelled.Load() {
			// 惰性取消：从链表移除，不执行回调
			s.cancelledCount.Add(1)
			s.taskCount.Add(-1)
			if current.key != "" {
				// 清理 key 索引（仅当索引仍指向此任务时）
				keyIndex.CompareAndDelete(current.key, current)
			}
			current = next
			continue
		}

		if current.rounds > 0 {
			// 未到期，圈数减一，保留到下一轮
			current.rounds--
			if keepHead == nil {
				keepHead = current
				keepTail = current
			} else {
				keepTail.next = current
				current.prev = keepTail
				keepTail = current
			}
			current = next
			continue
		}

		// 到期，执行回调
		s.completedCount.Add(1)
		s.taskCount.Add(-1)
		if current.key != "" {
			// 清理已执行任务的 key 索引
			keyIndex.CompareAndDelete(current.key, current)
		}
		task := current
		parent.executor(task.callback)
		current = next
	}

	// Phase 3：持锁挂回未到期任务（合并处理期间新 Schedule 插入的任务）
	if keepHead != nil {
		s.mu.Lock()
		keepTail.next = s.buckets[pos]
		if s.buckets[pos] != nil {
			s.buckets[pos].prev = keepTail
		}
		s.buckets[pos] = keepHead
		s.mu.Unlock()
	}
}

// schedule 在当前 shard 上调度任务（内部方法）
// 持 shard.mu 读取 currentTick 并头插，与 worker 推进 currentTick 互斥，
// 保证 bucketIdx 落在 worker 尚未扫过的区间，消除"延迟整圈"竞态
func (s *wheelShard) schedule(delay time.Duration, callback func(), key string, shardIdx int) *TimerTask {
	// 向上取整计算 tick 数（保证不提前触发）
	ticks := int((delay + s.tickInterval - 1) / s.tickInterval)
	if ticks < 1 {
		ticks = 1
	}

	task := &TimerTask{
		key:      key,
		callback: callback,
		shardIdx: shardIdx,
		wheel:    s.parent,
		// bucketIdx / rounds 在持锁内填入
	}

	// 持锁读取 currentTick + 计算 + 头插（同一临界区，消除竞态）
	s.mu.Lock()
	task.bucketIdx = (s.currentTick + ticks) & s.mask
	task.rounds = int64(ticks / s.bucketCount)
	head := s.buckets[task.bucketIdx]
	task.next = head
	if head != nil {
		head.prev = task
	}
	s.buckets[task.bucketIdx] = task
	s.mu.Unlock()

	s.taskCount.Add(1)
	return task
}

// Schedule 调度一个无 key 的定时任务（轮询分配 shard）
func (t *HashedWheelTimer) Schedule(delay time.Duration, task func()) *TimerTask {
	if t.stopped.Load() || delay <= 0 || task == nil {
		return nil
	}
	idx := int(uint64(t.rrCounter.Add(1))) & t.mask
	return t.shards[idx].schedule(delay, task, "", idx)
}

// ScheduleWithKey 调度一个带 key 的定时任务（同 key 覆盖会惰性取消旧任务）
func (t *HashedWheelTimer) ScheduleWithKey(key string, delay time.Duration, task func()) *TimerTask {
	if t.stopped.Load() || delay <= 0 || task == nil || key == "" {
		return nil
	}
	idx := int(FNVHashString32(key)) & t.mask
	tt := t.shards[idx].schedule(delay, task, key, idx)
	if tt != nil {
		// 原子交换：新任务入索引，旧任务（若存在）惰性取消
		if oldTask, loaded := t.keyIndex.Swap(key, tt); loaded && oldTask != nil {
			oldTask.cancelled.Store(true)
		}
	}
	return tt
}

// Refresh 刷新任务（O(1) 心跳刷新：取消旧 + 调度新）
func (t *HashedWheelTimer) Refresh(key string, delay time.Duration, task func()) *TimerTask {
	return t.ScheduleWithKey(key, delay, task)
}

// Cancel 取消指定任务（惰性取消，O(1)）
func (t *HashedWheelTimer) Cancel(tt *TimerTask) {
	if tt == nil || tt.wheel != t {
		return
	}
	tt.cancelled.Store(true)
	if tt.key != "" {
		// CAS 删除：仅当索引仍指向此任务时删除（避免误删 Refresh 新建的任务）
		t.keyIndex.CompareAndDelete(tt.key, tt)
	}
}

// CancelByKey 按 key 取消任务（惰性取消，O(1)）
func (t *HashedWheelTimer) CancelByKey(key string) {
	if key == "" {
		return
	}
	if oldTask, loaded := t.keyIndex.LoadAndDelete(key); loaded && oldTask != nil {
		oldTask.cancelled.Store(true)
	}
}

// Stop 停止时间轮（停止所有 worker，不再执行待触发任务）
func (t *HashedWheelTimer) Stop() {
	if !t.stopped.CompareAndSwap(false, true) {
		return
	}
	t.started.Store(false)
	for _, shard := range t.shards {
		close(shard.stopCh)
	}
	for _, shard := range t.shards {
		shard.wg.Wait()
	}
}

// Stats 返回时间轮统计信息
// completed/cancelled 为各 shard 计数器之和（下沉到 shard 避免全局原子竞争）
func (t *HashedWheelTimer) Stats() TimerStats {
	var active, completed, cancelled int64
	for _, shard := range t.shards {
		active += shard.taskCount.Load()
		completed += shard.completedCount.Load()
		cancelled += shard.cancelledCount.Load()
	}
	return TimerStats{
		ActiveTasks:    active,
		CompletedTasks: completed,
		CancelledTasks: cancelled,
	}
}
