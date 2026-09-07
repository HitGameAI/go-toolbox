/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-08-25 00:50:58
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-08-25 00:50:58
 * @FilePath: \go-toolbox\pkg\mathx\ratio_test.go
 * @Description: 比率计算与格式化测试
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */
package mathx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRatio(t *testing.T) {
	tests := []struct {
		name     string
		part     uint64
		total    uint64
		expected float64
	}{
		{"零分母", 5, 0, 0},
		{"全零", 0, 0, 0},
		{"分子为零", 0, 100, 0},
		{"半数", 50, 100, 0.5},
		{"全部", 100, 100, 1.0},
		{"三分之一", 1, 3, 1.0 / 3.0},
		{"大数", 999999, 1000000, 0.999999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, Ratio(tt.part, tt.total))
		})
	}
}

func TestRatioRound(t *testing.T) {
	tests := []struct {
		name      string
		part      int64
		total     int64
		precision int
		expected  float64
	}{
		{"零分母", 5, 0, 6, 0},
		{"全零", 0, 0, 6, 0},
		{"分子为零", 0, 100, 6, 0},
		{"整除", 50, 100, 6, 0.5},
		{"除不尽保留六位", 1, 3, 6, 0.333333},
		{"四舍五入进位", 2, 3, 6, 0.666667},
		{"零精度", 2, 3, 0, 1},
		{"负分子", -50, 100, 6, -0.5},
		{"大金额单位成本", 123456789, 7, 6, 17636684.142857},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, RatioRound(tt.part, tt.total, tt.precision))
		})
	}
}

func TestRatioFloat(t *testing.T) {
	tests := []struct {
		name     string
		part     float64
		total    float64
		expected float64
	}{
		{"零分母", 5.0, 0, 0},
		{"半数", 50.0, 100.0, 0.5},
		{"负数分子", -50.0, 100.0, -0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, RatioFloat(tt.part, tt.total))
		})
	}
}

func TestFormatRatio(t *testing.T) {
	tests := []struct {
		name      string
		ratio     float64
		precision int
		expected  string
	}{
		{"零", 0, 2, "0.00"},
		{"半数两位", 0.5, 2, "50.00"},
		{"半数零位", 0.5, 0, "50"},
		{"三位小数", 0.8555, 2, "85.55"},
		{"全数", 1.0, 2, "100.00"},
		{"三分之一两位", 1.0 / 3.0, 2, "33.33"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, FormatRatio(tt.ratio, tt.precision))
		})
	}
}

func TestFormatRatioFromCount(t *testing.T) {
	tests := []struct {
		name      string
		part      uint64
		total     uint64
		precision int
		expected  string
	}{
		{"零分母", 5, 0, 2, "0.00"},
		{"半数", 50, 100, 2, "50.00"},
		{"三分之一", 1, 3, 2, "33.33"},
		{"全数", 100, 100, 2, "100.00"},
		{"三位小数精度", 855, 1000, 3, "85.500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, FormatRatioFromCount(tt.part, tt.total, tt.precision))
		})
	}
}

// TestRatioSumEqualsOne 验证三率之和 = 1（业务场景：success+failed+cancelled = terminal）
func TestRatioSumEqualsOne(t *testing.T) {
	success, failed, cancelled := uint64(70), uint64(20), uint64(10)
	terminal := success + failed + cancelled
	sum := Ratio(success, terminal) + Ratio(failed, terminal) + Ratio(cancelled, terminal)
	assert.InDelta(t, 1.0, sum, 1e-9)
}
