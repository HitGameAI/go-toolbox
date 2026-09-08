/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2025-11-21 00:00:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2025-11-21 21:30:00
 * @FilePath: \go-toolbox\pkg\idgen\shortflake.go
 * @Description: ShortFlake 短 ID 生成器（高性能、紧凑型）
 *
 * Copyright (c) 2024 by kamalyes, All Rights Reserved.
 */

package idgen

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// ShortFlakeGenerator 短 Snowflake 生成器
// 生成 53 位整数（JavaScript Number 安全范围）
// 时间戳(41位) + 机器ID(6位) + 序列号(6位) = 53位
// 最大值: 9007199254740991 (约 9PB，16位数字)
type ShortFlakeGenerator struct {
	epoch   int64  // 自定义纪元（毫秒）
	nodeID  int64  // 节点ID (0-63)
	state   atomic.Uint64 // 无锁状态：高位 epoch 起毫秒数，低 6 位同毫秒序列号
	counter uint64
}

// NewShortFlakeGenerator 创建短 Snowflake 生成器
// nodeID: 0-63 (支持64个节点)
func NewShortFlakeGenerator(nodeID int64) *ShortFlakeGenerator {
	return &ShortFlakeGenerator{
		epoch:  1640995200000, // 2022-01-01 00:00:00
		nodeID: nodeID & 0x3F, // 6位，最大63
	}
}

// GenerateTraceID 生成跟踪ID（hex 编码，13字符）
// 格式: hex(shortflake_id)
// 示例: "018f5a3c7d2e4"
// 与 RequestID 的区别: hex 编码更紧凑，适合 HTTP Header 传递
func (g *ShortFlakeGenerator) GenerateTraceID() string {
	id := g.generate()
	return fmt.Sprintf("%013x", id)
}

// GenerateSpanID 生成跨度ID（hex 低8字符）
// 格式: hex(shortflake_id & 0xFFFFFFFF)
// 示例: "7d2e4b10"
// 与 TraceID 的区别: 更短，仅保留低位部分
func (g *ShortFlakeGenerator) GenerateSpanID() string {
	id := g.generate()
	return fmt.Sprintf("%08x", id&0xFFFFFFFF)
}

// GenerateRequestID 生成请求ID（纯数字+计数器后缀）
// 格式: shortflake数字-递增计数器
// 示例: "3425234523452-1"
// 与 TraceID 的区别: 纯数字+计数器，方便日志检索和排序
func (g *ShortFlakeGenerator) GenerateRequestID() string {
	counter := atomic.AddUint64(&g.counter, 1)
	id := g.generate()

	var sb strings.Builder
	sb.Grow(24)
	sb.WriteString(strconv.FormatInt(id, 10))
	sb.WriteByte('-')
	sb.WriteString(strconv.FormatUint(counter, 10))

	return sb.String()
}

// GenerateCorrelationID 生成关联ID（UUID 风格的 hex 格式）
// 格式: 8-4-4-4-4 hex（由 shortflake ID 拆分 + 计数器填充）
// 示例: "018f5a3c-7d2e-4b10-a1c3-e5f67890"
// 与 TraceID 的区别: UUID 格式，含连字符，适合跨系统传递
func (g *ShortFlakeGenerator) GenerateCorrelationID() string {
	id1 := g.generate()
	id2 := g.generate()
	h1 := fmt.Sprintf("%016x", id1)
	h2 := fmt.Sprintf("%016x", id2)

	h1 = h1[:12] + "4" + h1[13:]
	h2 = "8" + h2[1:]

	var buf [36]byte
	copy(buf[0:8], h1[0:8])
	buf[8] = '-'
	copy(buf[9:13], h1[8:12])
	buf[13] = '-'
	copy(buf[14:18], h1[12:16])
	buf[18] = '-'
	copy(buf[19:23], h2[0:4])
	buf[23] = '-'
	copy(buf[24:36], h2[4:16])

	return string(buf[:])
}

// Generate 生成 53 位整数 ID
func (g *ShortFlakeGenerator) Generate() int64 {
	return g.generate()
}

// generate 内部生成方法（无锁 CAS 实现）
// state 布局：高位为 epoch 起的毫秒数，低 6 位为同毫秒序列号
// 同毫秒序列耗尽时 CAS 自旋等待下一毫秒，不持有任何锁，其余调用方不受阻
func (g *ShortFlakeGenerator) generate() int64 {
	for {
		old := g.state.Load()
		now := uint64(time.Now().UnixMilli() - g.epoch)

		seq := uint64(0)
		if old>>6 == now {
			// 同一毫秒内，序列号递增
			seq = (old & 0x3F) + 1
			if seq > 0x3F {
				// 序列号溢出（64/ms 用尽），等待下一毫秒
				// 旧实现在持锁状态下忙等，会阻塞所有排队者；此处无锁自旋，天然让出
				runtime.Gosched()
				continue
			}
		}

		newState := (now << 6) | seq
		if g.state.CompareAndSwap(old, newState) {
			// 时间戳(41位) + 节点ID(6位) + 序列号(6位)
			return (int64(now) << 12) | (g.nodeID << 6) | int64(seq)
		}
		// CAS 失败：并发竞争，重试
	}
}

// ShortFlakeBase62Generator Base62 编码的短 ID 生成器
// 将 53 位数字编码为 9-10 字符的字符串
type ShortFlakeBase62Generator struct {
	*ShortFlakeGenerator
}

// NewShortFlakeBase62Generator 创建 Base62 编码生成器
func NewShortFlakeBase62Generator(nodeID int64) *ShortFlakeBase62Generator {
	return &ShortFlakeBase62Generator{
		ShortFlakeGenerator: NewShortFlakeGenerator(nodeID),
	}
}

const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// GenerateTraceID 生成 Base62 编码的 TraceID（9-10字符）
// 格式: base62(shortflake_id)
// 示例: "aB3xK9mPqR"
// 与 RequestID 的区别: Base62 编码更短，URL 安全
func (g *ShortFlakeBase62Generator) GenerateTraceID() string {
	return g.encodeBase62(g.generate())
}

// GenerateSpanID 生成 Base62 编码的 SpanID（6字符）
// 格式: base62(shortflake_id & 0x3FFFFFFFFF)，取低36位
// 示例: "K9mPxR"
// 与 TraceID 的区别: 更短，仅保留低位部分
func (g *ShortFlakeBase62Generator) GenerateSpanID() string {
	id := g.generate()
	return g.encodeBase62(id & 0x3FFFFFFFFF)
}

// GenerateRequestID 生成 Base62 编码的 RequestID（前缀+计数器）
// 格式: base62前6字符-递增计数器
// 示例: "aB3xK9-1"
// 与 TraceID 的区别: 带计数器后缀，可排序
func (g *ShortFlakeBase62Generator) GenerateRequestID() string {
	counter := atomic.AddUint64(&g.counter, 1)
	id := g.generate()
	prefix := g.encodeBase62(id)

	var sb strings.Builder
	sb.Grow(16)
	if len(prefix) >= 6 {
		sb.WriteString(prefix[:6])
	} else {
		sb.WriteString(prefix)
	}
	sb.WriteByte('-')
	sb.WriteString(strconv.FormatUint(counter, 10))

	return sb.String()
}

// GenerateCorrelationID 生成 Base62 编码的 CorrelationID（双段拼接）
// 格式: base62(shortflake_id)-base62(shortflake_id)，18-22字符
// 示例: "aB3xK9mPqR-xY7wN4qL2v"
// 与 TraceID 的区别: 双段拼接，增强唯一性，适合跨系统关联
func (g *ShortFlakeBase62Generator) GenerateCorrelationID() string {
	id1 := g.generate()
	id2 := g.generate()

	var sb strings.Builder
	sb.Grow(22)
	sb.WriteString(g.encodeBase62(id1))
	sb.WriteByte('-')
	sb.WriteString(g.encodeBase62(id2))

	return sb.String()
}

// encodeBase62 将整数编码为 Base62 字符串（9-10字符）
func (g *ShortFlakeBase62Generator) encodeBase62(num int64) string {
	if num == 0 {
		return "0"
	}

	var result [11]byte
	pos := len(result) - 1

	for num > 0 {
		result[pos] = base62Chars[num%62]
		num /= 62
		pos--
	}

	return string(result[pos+1:])
}
