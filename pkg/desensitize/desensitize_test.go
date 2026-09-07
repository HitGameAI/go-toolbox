/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2023-07-28 00:50:58
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2024-12-19 11:19:10
 * @FilePath: \go-toolbox\pkg\desensitize\desensitize_test.go
 * @Description:
 *
 * Copyright (c) 2024 by kamalyes, All Rights Reserved.
 */
package desensitize

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSensitiveData(t *testing.T) {
	assert.Equal(t, "1***", SensitiveData("1234", 0, 0))
	assert.Equal(t, "1***", SensitiveData("1234", 2, 2))
	assert.Equal(t, "12*4", SensitiveData("1234", 2, 3))
	assert.Equal(t, "1***", SensitiveData("1234", 0, 5))
	assert.Equal(t, "", SensitiveData("", 0, 0))
}

func TestDesensitizeAllTypes(t *testing.T) {
	desensitizeOptions := NewDesensitizeOptions()
	testCases := map[string]struct {
		input           string
		expected        string
		desensitizeType DesensitizeType
		option          DesensitizeOptions
	}{
		"TestSensitiveData": {
			input:           "123456789",
			expected:        "123****89",
			desensitizeType: CustomExtension,
			option:          DesensitizeOptions{CustomExtensionStartIndex: 3, CustomExtensionEndIndex: 7},
		},
		"TestChineseName": {
			input:           "李四",
			expected:        "李*",
			desensitizeType: ChineseName,
		},
		"TestIDCard": {
			input:           "123456789012345678",
			expected:        "123456********5678",
			desensitizeType: IDCard,
			option:          desensitizeOptions,
		},
		"TestPassWord": {
			input:           "12678",
			expected:        "1****",
			desensitizeType: Password,
		},
		"TestPhoneNumber": {
			input:           "18175698789",
			expected:        "181****8789",
			desensitizeType: PhoneNumber,
			option:          desensitizeOptions,
		},
		"TestMobilePhone": {
			input:           "13812345678",
			expected:        "138****5678",
			desensitizeType: MobilePhone,
			option:          desensitizeOptions,
		},
		"TestAddress": {
			input:           "北京市朝阳区某某街道123号",
			expected:        "北京市朝*******23号",
			desensitizeType: Address,
			option:          desensitizeOptions,
		},
		"TestEmail": {
			input:           "example@test.com",
			expected:        "e****le@test.com",
			desensitizeType: Email,
			option:          desensitizeOptions,
		},
		"TestCarLicense": {
			input:           "浙A12345B",
			expected:        "浙A1***5B",
			desensitizeType: CarLicense,
		},
		"TestBankCard": {
			input:           "1234567890123456",
			expected:        "1234 **** **** **** 3456",
			desensitizeType: BankCard,
			option:          desensitizeOptions,
		},
		"TestIPv4": {
			input:           "192.168.1.1",
			expected:        "192.*.*.*",
			desensitizeType: IPV4,
		},
		"TestIPv6": {
			input:           "2001:0db8:86a3:08d3:1319:8a2e:0370:7344",
			expected:        "2001:*:*:*:*:*:*:*",
			desensitizeType: IPV6,
		},
		"TestPEMKey": {
			input: strings.Join([]string{
				"-----BEGIN PRIVATE KEY-----",
				"1234567890abcdef1234567890abcdef12345678",
				"-----END PRIVATE KEY-----",
			}, "\r\n"),
			expected: strings.Join([]string{
				"-----BEGIN PRIVATE KEY-----",
				"1234567890abcdef********90abcdef12345678",
				"-----END PRIVATE KEY-----",
			}, "\n"),
			desensitizeType: PEMKey,
			option:          desensitizeOptions,
		},
		"TestUserID": {
			input:           "1000234567",
			expected:        "10******67",
			desensitizeType: UserID,
			option:          desensitizeOptions,
		},
		"TestPlayerID": {
			input:           "P202400123",
			expected:        "P2******23",
			desensitizeType: PlayerID,
			option:          desensitizeOptions,
		},
		"TestOrderNo": {
			input:           "ORD2024010112345",
			expected:        "OR************45",
			desensitizeType: OrderNo,
			option:          desensitizeOptions,
		},
		"TestUUID": {
			input:           "550e8400-e29b-41d4-a716-446655440000",
			expected:        "55" + strings.Repeat("*", 32) + "00",
			desensitizeType: UUID,
			option:          desensitizeOptions,
		},
		"TestOpenID": {
			input:           "o6_bmjrPTlm6_2sgVt7hMZ3",
			expected:        "o6" + strings.Repeat("*", 19) + "Z3",
			desensitizeType: OpenID,
			option:          desensitizeOptions,
		},
		"TestAccount": {
			input:           "zhangsan",
			expected:        "zh****an",
			desensitizeType: Account,
			option:          desensitizeOptions,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, Desensitize(tc.input, tc.desensitizeType, tc.option))
		})
	}
}

func TestPhoneNumber(t *testing.T) {
	assert.Equal(t, "", SensitizePhoneNumber("", 3, 7))
	assert.Equal(t, "181****8789", SensitizePhoneNumber("18175698789", 3, 7))
	assert.Equal(t, "181*****789", SensitizePhoneNumber("1817789", 3, 7))
}

func TestBankCard(t *testing.T) {
	assert.Equal(t, "1234 **** **** *** 6789", SensitizeBankCard("123456789", 16))
	assert.Equal(t, "1234 **** **** *** 6789", SensitizeBankCard("1234 5678 9", 16))
	assert.Equal(t, "1234 **** **** **** 6789", SensitizeBankCard("123456789", 19))
	assert.Equal(t, "1234 **** **** **** 6789", SensitizeBankCard("1234 5678 9", 19))
	assert.Equal(t, "", SensitizeBankCard("", 19))
}

func TestIPv4(t *testing.T) {
	assert.Equal(t, "127.*.*.*", SensitizeIpv4("127.0.0.1"))
	assert.Equal(t, "", SensitizeIpv4(""))
}

func TestIPv6(t *testing.T) {
	assert.Equal(t, "2001:*:*:*:*:*:*:*", SensitizeIpv6("2001:0db8:86a3:08d3:1319:8a2e:0370:7344"))
	assert.Equal(t, "", SensitizeIpv6(""))
}

func TestSensitizePEMKey(t *testing.T) {
	assert.Equal(t, strings.Repeat("*", len("short-secret")), SensitizePEMKey("short-secret", 16, 16))
}

func TestDesensitizeIDCustomOptions(t *testing.T) {
	opt := NewDesensitizeOptions()
	opt.IDPrefixVisibleLen = 4
	opt.IDSuffixVisibleLen = 4
	assert.Equal(t, "1000**4567", Desensitize("1000234567", UserID, opt))
	assert.Equal(t, "ORD2****2345", Desensitize("ORD2024010112345", OrderNo, opt))

	// 短字符串：前后缀可见长度之和大于等于长度时全掩码
	assert.Equal(t, "****", Desensitize("1234", UserID, NewDesensitizeOptions()))
	// 空字符串直接返回
	assert.Equal(t, "", Desensitize("", PlayerID))
}
