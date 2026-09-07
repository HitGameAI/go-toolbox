/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2023-07-28 00:50:58
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2024-12-19 08:15:19
 * @FilePath: \go-toolbox\pkg\desensitize\model.go
 * @Description:
 *
 * Copyright (c) 2024 by kamalyes, All Rights Reserved.
 */
package desensitize

type DesensitizeType int

// DesensitizeOptions 脱敏选项
type DesensitizeOptions struct {
	CustomExtensionStartIndex int
	CustomExtensionEndIndex   int
	ChineseNameStartIndex     int
	IdCardStartIndex          int
	IdCardLength              int
	PhoneNumberStartIndex     int
	PhoneNumberEndIndex       int
	MobilePhoneStartIndex     int
	EmailStartIndex           int
	PEMBodyPrefixVisibleLen   int
	PEMBodySuffixVisibleLen   int
	APIKeyPrefixVisibleLen    int
	APIKeySuffixVisibleLen    int
	SecretPrefixVisibleLen    int
	SecretSuffixVisibleLen    int
	IDPrefixVisibleLen        int // ID 类（用户ID/玩家ID/订单号/UUID/OpenID/账号）前缀可见长度
	IDSuffixVisibleLen        int // ID 类后缀可见长度
}

// NewDesensitizeOptions 创建带有默认值的 DesensitizeOptions
func NewDesensitizeOptions() DesensitizeOptions {
	return DesensitizeOptions{
		CustomExtensionStartIndex: 1,
		CustomExtensionEndIndex:   1,
		ChineseNameStartIndex:     1,
		IdCardStartIndex:          6,
		IdCardLength:              19,
		PhoneNumberStartIndex:     3,
		PhoneNumberEndIndex:       7,
		MobilePhoneStartIndex:     3,
		EmailStartIndex:           1,
		PEMBodyPrefixVisibleLen:   16,
		PEMBodySuffixVisibleLen:   16,
		APIKeyPrefixVisibleLen:    4,
		APIKeySuffixVisibleLen:    4,
		SecretPrefixVisibleLen:    4,
		SecretSuffixVisibleLen:    4,
		IDPrefixVisibleLen:        2,
		IDSuffixVisibleLen:        2,
	}
}
