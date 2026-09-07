/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2023-07-28 00:50:58
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2024-12-19 08:15:19
 * @FilePath: \go-toolbox\pkg\desensitize\desensitize.go
 * @Description:
 *
 * Copyright (c) 2024 by kamalyes, All Rights Reserved.
 */

package desensitize

import (
	"reflect"
	"strings"
	"unicode/utf8"

	validator "github.com/kamalyes/go-argus"
	"github.com/kamalyes/go-toolbox/pkg/stringx"
)

// Desensitize 数据脱敏
func Desensitize(str string, DesensitizeType DesensitizeType, options ...DesensitizeOptions) string {
	// 如果数据为空，则返回空字符串
	if validator.IsEmptyValue(reflect.ValueOf(str)) {
		return str
	}
	opt := NewDesensitizeOptions()
	if len(options) > 0 {
		opt = options[0]
	}
	var newStr string
	switch DesensitizeType {
	case CustomExtension:
		newStr = SensitiveData(str, opt.CustomExtensionStartIndex, opt.CustomExtensionEndIndex)
	case ChineseName:
		newStr = SensitiveData(str, opt.ChineseNameStartIndex, 1+(stringx.Length(str)-2)/2)
	case IDCard:
		newStr = SensitiveData(str, opt.IdCardStartIndex, stringx.Length(str)-4)
	case PhoneNumber:
		newStr = SensitizePhoneNumber(str, opt.PhoneNumberStartIndex, opt.PhoneNumberEndIndex)
	case MobilePhone:
		newStr = SensitiveData(str, opt.MobilePhoneStartIndex, stringx.Length(str)-4)
	case Address:
		newStr = SensitiveData(str, stringx.Length(str)/3, stringx.Length(str)-3)
	case Email:
		newStr = SensitiveData(str, opt.EmailStartIndex, stringx.IndexOf(str, "@")-2)
	case Password:
		newStr = SensitiveData(str, 0, stringx.Length(str))
	case CarLicense:
		newStr = SensitiveData(str, 3, stringx.Length(str)-2)
	case BankCard:
		newStr = SensitizeBankCard(str, opt.IdCardLength)
	case IPV4:
		newStr = SensitizeIpv4(str)
	case IPV6:
		newStr = SensitizeIpv6(str)
	case PEMKey:
		newStr = SensitizePEMKey(str, opt.PEMBodyPrefixVisibleLen, opt.PEMBodySuffixVisibleLen)
	case APIKey:
		newStr = sensitizeSecretSegment(str, opt.APIKeyPrefixVisibleLen, opt.APIKeySuffixVisibleLen)
	case Secret:
		newStr = sensitizeSecretSegment(str, opt.SecretPrefixVisibleLen, opt.SecretSuffixVisibleLen)
	case UserID, PlayerID, OrderNo, UUID, OpenID, Account:
		newStr = sensitizeSecretSegment(str, opt.IDPrefixVisibleLen, opt.IDSuffixVisibleLen)
	default:
		newStr = str
	}

	return newStr
}

// 通用脱敏函数
func SensitiveData(str string, start, end int) string {
	// 如果数据为空，则返回空字符串
	if validator.IsEmptyValue(reflect.ValueOf(str)) {
		return str
	}

	// 获取字符数量
	charCount := utf8.RuneCountInString(str)

	// 调整起始位置和结束位置
	if start <= 0 || start >= charCount {
		start = 1
	}

	if end <= 0 || end >= charCount {
		end = charCount
	}

	// 如果起始位置和结束位置相等，则视为需要处理整个文本
	if start == end {
		start = 1
		end = charCount
	}

	// 对指定位置的敏感数据进行隐藏处理
	return stringx.Hide(str, start, end)
}

// SensitizeJump 跳步脱敏：掩码 [start, end) 区间内从 start 开始、以 step 为步长跳步选取的字符
// start：开始索引（含），支持负数（-1 表示最后一个字符，按实际长度动态换算）
// end：结束索引（不含），支持负数动态换算（-N 表示保留末尾 N 个字符，0 表示到末尾）
// step：跳步数（<=0 时按 1 处理，1 为连续掩码，2 为隔位掩码）
func SensitizeJump(str string, start, end, step int) string {
	// 如果数据为空，则返回空字符串
	if validator.IsEmptyValue(reflect.ValueOf(str)) {
		return str
	}

	// 获取字符数量
	charCount := utf8.RuneCountInString(str)

	// 负数开始索引按实际长度动态换算为绝对索引
	if start < 0 {
		start += charCount
	}

	// 结束索引 <=0 时按实际长度动态换算：-N 即保留末尾 N 个字符，0 表示到末尾
	if end <= 0 {
		end += charCount
	}

	// 边界修正
	if start < 0 {
		start = 0
	}
	if end > charCount {
		end = charCount
	}

	// 开始位置不小于结束位置时无需脱敏
	if start >= end {
		return str
	}

	// 跳步数非法时按连续掩码处理
	if step <= 0 {
		step = 1
	}

	// 按跳步掩码指定位置的字符
	runes := []rune(str)
	for i := start; i < end; i += step {
		runes[i] = '*'
	}
	return string(runes)
}

// 手机号脱敏
func SensitizePhoneNumber(str string, start, end int) string {
	// 空判断
	if validator.IsEmptyValue(reflect.ValueOf(str)) {
		return str
	}
	return SensitiveData(stringx.Pad(str, 11), start, end)
}

// 银行卡号脱敏
func SensitizeBankCard(str string, cardLength int) string {
	// 如果卡号为空，则直接返回原卡号
	if validator.IsEmptyValue(reflect.ValueOf(str)) {
		return str
	}

	// 清理卡号中的空格
	cleanCardNo := stringx.CleanEmpty(str)
	// 使用指定的长度填充卡号
	cleanCardNo = stringx.Pad(cleanCardNo, cardLength)
	// 获取卡号长度
	length := stringx.Length(cleanCardNo)
	// 默认间隔为4
	interval := 4

	// 根据卡号长度和特殊情况调整末尾长度
	endLength := length % interval
	if cardLength == 16 {
		endLength = length % 3
	}
	// 计算中间部分长度
	midLength := length - interval - endLength

	// 生成新的卡号切片，初始包含前4位
	newCardNo := []rune(cleanCardNo[:interval])
	for i := 0; i < midLength; i++ {
		// 每隔4位插入一个空格
		if i%interval == 0 {
			newCardNo = append(newCardNo, ' ')
		}
		// 其余字符用*替换
		newCardNo = append(newCardNo, '*')
	}
	// 在中间部分的最末尾插入一个空格
	newCardNo = append(newCardNo, ' ')
	// 添加卡号的最后4位
	lastFour := cleanCardNo[stringx.Length(cleanCardNo)-4:]
	newCardNo = append(newCardNo, []rune(lastFour)...)

	// 返回格式化后的卡号
	return string(newCardNo)
}

// ipv4 脱敏
func SensitizeIpv4(str string) string {
	if validator.IsEmptyValue(reflect.ValueOf(str)) {
		return str
	}
	newIP := stringx.SubBefore(str, ".", false)
	return newIP + ".*.*.*"
}

// ipv6 脱敏
func SensitizeIpv6(str string) string {
	if validator.IsEmptyValue(reflect.ValueOf(str)) {
		return str
	}
	newIP := stringx.SubBefore(str, ":", false)
	return newIP + ":*:*:*:*:*:*:*"
}

// PEM 密钥脱敏
func SensitizePEMKey(str string, prefixVisibleLen, suffixVisibleLen int) string {
	if validator.IsEmptyValue(reflect.ValueOf(str)) {
		return str
	}

	normalized := strings.TrimSpace(strings.ReplaceAll(str, "\r\n", "\n"))
	if normalized == "" {
		return str
	}

	lines := strings.Split(normalized, "\n")
	if len(lines) < 3 || !strings.HasPrefix(lines[0], "-----BEGIN ") || !strings.HasPrefix(lines[len(lines)-1], "-----END ") {
		return sensitizeSecretSegment(normalized, prefixVisibleLen, suffixVisibleLen)
	}

	body := strings.Join(lines[1:len(lines)-1], "")
	return strings.Join([]string{
		lines[0],
		sensitizeSecretSegment(body, prefixVisibleLen, suffixVisibleLen),
		lines[len(lines)-1],
	}, "\n")
}

func sensitizeSecretSegment(secret string, prefixVisibleLen, suffixVisibleLen int) string {
	charCount := utf8.RuneCountInString(secret)
	if charCount == 0 {
		return secret
	}

	if prefixVisibleLen < 0 {
		prefixVisibleLen = 0
	}
	if suffixVisibleLen < 0 {
		suffixVisibleLen = 0
	}

	if prefixVisibleLen+suffixVisibleLen >= charCount {
		return strings.Repeat("*", charCount)
	}

	return stringx.Hide(secret, prefixVisibleLen, charCount-suffixVisibleLen)
}
