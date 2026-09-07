/*
 * @Author: kamalyes 501893067@qq.com
 * @Date:2024-12-18 22:53:55
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2024-12-19 08:15:19
 * @FilePath: \go-toolbox\pkg\desensitize\desensitize_adapter_test.go
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

// 定义一个测试结构体
type TestJsonDesensitizationStruct struct {
	Email       string `desensitize:"email"`
	PhoneNumber string `desensitize:"phoneNumber"`
	Name        string `desensitize:"name"`
	Age         int
}

// 定义 ID 类脱敏测试结构体
type TestIDDesensitizationStruct struct {
	UserID     string `desensitize:"userId"`
	PlayerID   string `desensitize:"playerId"`
	OpenID     string `desensitize:"openId"`
	UnionID    string `desensitize:"unionId"`
	OrderNo    string `desensitize:"orderNo"`
	DeviceUUID string `desensitize:"uuid"`
	Account    string `desensitize:"account"`
}

// 自定义脱敏器示例
type MyCustomDesensitizer struct{}

func (d *MyCustomDesensitizer) Desensitize(value string) string {
	// 自定义脱敏逻辑
	return "*****" // 示例：返回固定的脱敏值
}

func init() {
	// 注册自定义脱敏器
	RegisterDesensitizer("myCustom", &MyCustomDesensitizer{})
}

func TestDesensitization(t *testing.T) {
	// 创建一个测试对象
	testObj := &TestJsonDesensitizationStruct{
		Email:       "test@example.com",
		PhoneNumber: "1234567890",
		Name:        "张三",
		Age:         30,
	}

	// 执行脱敏操作
	err := Desensitization(testObj)

	// 断言没有错误
	assert.NoError(t, err)

	// 断言脱敏后的结果
	assert.Equal(t, "t*st@example.com", testObj.Email)
	assert.Equal(t, "123****7890", testObj.PhoneNumber)
	assert.Equal(t, "张*", testObj.Name)
	assert.Equal(t, 30, testObj.Age) // 确保年龄没有被脱敏
}

func TestDesensitization_IDTypes(t *testing.T) {
	testObj := &TestIDDesensitizationStruct{
		UserID:     "1000234567",
		PlayerID:   "P202400123",
		OpenID:     "o6_bmjrPTlm6_2sgVt7hMZ3",
		UnionID:    "o6_bmjrPTlm6_2sgVt7hMZ3fQ",
		OrderNo:    "ORD2024010112345",
		DeviceUUID: "550e8400-e29b-41d4-a716-446655440000",
		Account:    "zhangsan",
	}

	err := Desensitization(testObj)

	assert.NoError(t, err)
	assert.Equal(t, "10******67", testObj.UserID)
	assert.Equal(t, "P2******23", testObj.PlayerID)
	assert.Equal(t, "o6"+strings.Repeat("*", 19)+"Z3", testObj.OpenID)
	assert.Equal(t, "o6"+strings.Repeat("*", 20)+"fQ", testObj.UnionID)
	assert.Equal(t, "OR************45", testObj.OrderNo)
	assert.Equal(t, "55"+strings.Repeat("*", 32)+"00", testObj.DeviceUUID)
	assert.Equal(t, "zh****an", testObj.Account)
}

func TestDesensitization_NonStruct(t *testing.T) {
	// 测试非结构体输入
	err := Desensitization("not a struct")
	assert.Error(t, err)
	assert.Equal(t, "expected a non-nil pointer to a struct", err.Error())
}

func TestDesensitization_EmptyDesensitizer(t *testing.T) {
	// 测试未注册脱敏器的情况
	type TestEmptyDesensitizerStruct struct {
		A string `desensitize:"abc"`
	}

	testObj := &TestEmptyDesensitizerStruct{
		A: "10",
	}

	err := Desensitization(testObj)
	assert.NoError(t, err)           // 不会报错
	assert.Equal(t, "10", testObj.A) // 应保持原值
}

func TestDesensitization_CustomDesensitizer(t *testing.T) {
	// 定义一个测试结构体，使用自定义脱敏器
	type TestCustomDesensitizationStruct struct {
		SecretField string `desensitize:"myCustom"`
	}

	// 创建一个测试对象
	testObj := &TestCustomDesensitizationStruct{
		SecretField: "SensitiveData",
	}

	// 执行脱敏操作
	err := Desensitization(testObj)

	// 断言没有错误
	assert.NoError(t, err)

	// 断言脱敏后的结果
	assert.Equal(t, "*****", testObj.SecretField) // 应该被自定义脱敏器处理
}

func TestDesensitization_Slice(t *testing.T) {
	// 定义一个包含切片的结构体
	type TestSliceStruct struct {
		Emails []string `desensitize:"email"`
	}

	testObj := &TestSliceStruct{
		Emails: []string{"user123@example.com", "a123568@example.com"},
	}

	err := Desensitization(testObj)
	assert.NoError(t, err)

	expectedEmails := []string{"u****23@example.com", "a****68@example.com"}
	assert.Equal(t, expectedEmails, testObj.Emails)
}

func TestDesensitization_Array(t *testing.T) {
	// 定义一个包含数组的结构体
	type TestArrayStruct struct {
		PhoneNumbers [2]string `desensitize:"phoneNumber"`
	}

	testObj := &TestArrayStruct{
		PhoneNumbers: [2]string{"1234567890", "0987654321"},
	}

	err := Desensitization(testObj)
	assert.NoError(t, err)

	expectedPhoneNumbers := [2]string{"123****7890", "098****4321"}
	assert.Equal(t, expectedPhoneNumbers, testObj.PhoneNumbers)
}

func TestDesensitization_Map(t *testing.T) {
	// 定义一个包含映射的结构体
	type TestMapStruct struct {
		Contacts map[string]string `desensitize:"phoneNumber"`
	}

	testObj := &TestMapStruct{
		Contacts: map[string]string{
			"John": "1234567890",
			"Jane": "098321",
		},
	}

	err := Desensitization(testObj)
	assert.NoError(t, err)

	expectedContacts := map[string]string{
		"John": "123****7890",
		"Jane": "098******21",
	}
	assert.Equal(t, expectedContacts, testObj.Contacts)
}

func TestDesensitization_ComplexStruct(t *testing.T) {

	// 定义嵌套结构体
	type Other struct {
		PhoneNumber string `desensitize:"phoneNumber"`
	}

	type UserProfile struct {
		Username string `desensitize:"name"`
		Email    string `desensitize:"email"`
		Other    Other
	}

	// 定义包含用户资料的复合结构体
	type ComplexStruct struct {
		Profiles []UserProfile
		Names    map[string]string `desensitize:"name"`
	}

	// 创建一个复杂结构体的测试对象
	testObj := &ComplexStruct{
		Profiles: []UserProfile{
			{
				Username: "jo",
				Email:    "john@example.com",
				Other: Other{
					PhoneNumber: "18169967587",
				},
			},
			{
				Username: "jane_doe",
				Email:    "ane12368@example.com",
				Other: Other{
					PhoneNumber: "18199687567",
				},
			},
		},
		Names: map[string]string{
			"theme": "dark",
			"lang":  "en",
		},
	}

	// 执行脱敏操作
	err := Desensitization(testObj)

	// 断言没有错误
	assert.NoError(t, err)

	// 断言脱敏后的结果
	assert.Equal(t, "j*", testObj.Profiles[0].Username)
	assert.Equal(t, "j*hn@example.com", testObj.Profiles[0].Email)
	assert.Equal(t, "181****7587", testObj.Profiles[0].Other.PhoneNumber)

	assert.Equal(t, "j***_doe", testObj.Profiles[1].Username)
	assert.Equal(t, "a*****68@example.com", testObj.Profiles[1].Email)
	assert.Equal(t, "181****7567", testObj.Profiles[1].Other.PhoneNumber)

	// Settings 应该保持原值，因为没有注册脱敏器
	assert.Equal(t, "d*rk", testObj.Names["theme"])
	assert.Equal(t, "e*", testObj.Names["lang"])
}
