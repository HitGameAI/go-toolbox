/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-08-25 00:50:58
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-08-25 00:50:58
 * @FilePath: \go-toolbox\pkg\mathx\ratio.go
 * @Description: 比率计算与格式化，用于统计场景的成功率/失败率/取消率等
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package mathx

import (
	"math"
	"strconv"
)

// Ratio 计算比率，返回 [0,1] 区间的小数
// part 为分子，total 为分母；total 为 0 时返回 0（避免除零）
// 与 Percentage 的区别：Percentage 返回 [0,100]，Ratio 返回 [0,1]
// 适用于"终态订单数 = success+failed+cancelled"作为分母的率计算
func Ratio(part, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total)
}

// RatioRound 计算比率并四舍五入保留 precision 位小数，分母为 0 时返回 0
// 适用于金额/计数等 int64 指标的安全占比计算，避免结果带浮点尾差
func RatioRound(part, total int64, precision int) float64 {
	if total == 0 {
		return 0
	}
	scale := math.Pow10(precision)
	return math.Round(float64(part)/float64(total)*scale) / scale
}

// RatioFloat 浮点版本，用于分子分母已是 float64 的场景
func RatioFloat(part, total float64) float64 {
	if total == 0 {
		return 0
	}
	return part / total
}

// FormatRatio 将 [0,1] 区间的比率格式化为百分比字符串（不带 % 后缀）
// precision 为小数位数，如 0.855 + precision=2 → "85.50"
// 内部先乘 100 再格式化，避免外部重复 *100 造成精度损失
func FormatRatio(ratio float64, precision int) string {
	return strconv.FormatFloat(ratio*100, 'f', precision, 64)
}

// FormatRatioFromCount 从 count 直接计算并格式化百分比字符串（不带 % 后缀）
// 等价于 FormatRatio(Ratio(part, total), precision)，但减少一次中间赋值
// part 为分子（如 success_count），total 为分母（如 终态订单数），precision 为小数位数
func FormatRatioFromCount(part, total uint64, precision int) string {
	return FormatRatio(Ratio(part, total), precision)
}
