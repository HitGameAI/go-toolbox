/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2024-12-18 18:50:58
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2024-12-19 08:15:19
 * @FilePath: \go-toolbox\pkg\desensitize\adapter.go
 * @Description:
 * 该文件实现了数据脱敏的功能，包括注册脱敏器和执行脱敏操作。
 *
 * Copyright (c) 2024 by kamalyes, All Rights Reserved.
 */
package desensitize

import (
	"errors"
	"reflect"
	"sync"

	"github.com/kamalyes/go-toolbox/pkg/syncx"
)

// Desensitizer 接口定义
// 该接口用于定义脱敏器的基本行为，包含一个脱敏方法。
type Desensitizer interface {
	Desensitize(value string) string // 输入一个值，返回脱敏后的值
}

// DefaultDesensitizer 默认脱敏适配器
// 该结构体实现了 Desensitizer 接口，用于处理标准的脱敏逻辑。
type DefaultDesensitizer struct {
	desensitizerType DesensitizeType // 脱敏类型
}

// Desensitize 方法实现
// 根据指定的脱敏类型对输入的值进行脱敏处理。
// @param value: 需要脱敏的字符串值。
// @returns
//   - 返回脱敏后的字符串值。
func (e *DefaultDesensitizer) Desensitize(value string) string {
	return Desensitize(value, e.desensitizerType) // 调用外部库进行脱敏
}

var desensitizers map[string]Desensitizer // 存储注册的脱敏器
var desensitizerMu sync.RWMutex           // 读写锁，用于保护脱敏器的并发访问

// RegisterDesensitizer 注册脱敏器（支持现有和自定义）
// @param desensitizerType: 脱敏器的类型标识。
// @param desensitizer: 实现了 Desensitizer 接口的脱敏器实例。
// @returns
//   - 无返回值。
func RegisterDesensitizer(desensitizerType string, desensitizer Desensitizer) {
	syncx.WithLock(&desensitizerMu, func() {
		if desensitizers == nil {
			desensitizers = make(map[string]Desensitizer) // 初始化脱敏器映射
		}
		desensitizers[desensitizerType] = desensitizer // 注册脱敏器
	})
}

// OperateByRule 根据规则进行脱敏操作
// @param desensitizerType: 脱敏器的类型标识。
// @param in: 需要脱敏的输入值，应该是字符串类型。
// @returns
//   - 返回脱敏后的值和可能的错误。
func OperateByRule(desensitizerType string, in interface{}) (interface{}, error) {
	return syncx.WithLockReturn(&desensitizerMu, func() (interface{}, error) {
		operator, ok := desensitizers[desensitizerType] // 查找对应的脱敏器
		if !ok {
			return nil, errors.New("desensitizer not found") // 未找到对应的脱敏器
		}
		return operator.Desensitize(in.(string)), nil // 执行脱敏操作
	})
}

// Desensitization 执行脱敏操作
// @param obj: 需要进行脱敏的对象，应该是结构体或指向结构体的指针。
// @returns
//   - 返回可能的错误。
func Desensitization(obj interface{}) error {
	// 获取传入对象的反射值
	fieldValue := reflect.ValueOf(obj)

	// 检查是否为非空指针
	if fieldValue.Kind() != reflect.Ptr || fieldValue.IsNil() {
		return errors.New("expected a non-nil pointer to a struct")
	}

	// 获取指针指向的值
	fieldValue = fieldValue.Elem()

	// 遍历结构体的每个字段
	for i := 0; i < fieldValue.NumField(); i++ {
		field := fieldValue.Type().Field(i) // 获取字段类型信息
		tag := field.Tag.Get("desensitize") // 获取字段的脱敏标签
		fieldType := fieldValue.Field(i)    // 获取字段的值

		// 处理字段的脱敏
		if err := processField(fieldType, tag); err != nil {
			return err
		}
	}
	return nil // 返回nil表示成功
}

// processField 处理字段的脱敏逻辑
func processField(fieldValue reflect.Value, tag string) error {
	if !fieldValue.CanSet() {
		return nil // 如果字段不可设置，直接返回
	}
	switch fieldValue.Kind() {
	case reflect.Slice, reflect.Array:
		// 如果字段是切片或数组，处理每个元素
		for j := 0; j < fieldValue.Len(); j++ {
			elemValue := fieldValue.Index(j)
			if elemValue.Kind() == reflect.Struct {
				// 如果元素是结构体，递归处理
				if err := Desensitization(elemValue.Addr().Interface()); err != nil {
					return err
				}
			} else {
				// 否则直接对每个元素应用脱敏规则
				newValue, err := OperateByRule(tag, elemValue.Interface())
				if err == nil {
					elemValue.Set(reflect.ValueOf(newValue)) // 更新元素值
				}
			}
		}
	case reflect.Struct:
		// 如果字段是结构体，递归处理
		return Desensitization(fieldValue.Addr().Interface())
	case reflect.Map:
		// 如果字段是映射，遍历每个键值对
		for _, key := range fieldValue.MapKeys() {
			value := fieldValue.MapIndex(key)
			newValue, err := OperateByRule(tag, value.Interface())
			if err == nil {
				fieldValue.SetMapIndex(key, reflect.ValueOf(newValue)) // 更新映射中的值
			}
		}
	default:
		// 对于其他类型，直接应用脱敏规则
		newValue, err := OperateByRule(tag, fieldValue.Interface())
		if err == nil {
			fieldValue.Set(reflect.ValueOf(newValue)) // 更新字段值
		}
	}
	return nil
}

// 初始化时注册现有的脱敏器
func init() {
	// 注册预定义的脱敏器
	RegisterDesensitizer("email", &DefaultDesensitizer{Email})
	RegisterDesensitizer("phoneNumber", &DefaultDesensitizer{PhoneNumber})
	RegisterDesensitizer("name", &DefaultDesensitizer{ChineseName})
	RegisterDesensitizer("identityCard", &DefaultDesensitizer{IDCard})
	RegisterDesensitizer("mobilePhone", &DefaultDesensitizer{MobilePhone})
	RegisterDesensitizer("address", &DefaultDesensitizer{Address})
	RegisterDesensitizer("password", &DefaultDesensitizer{Password})
	RegisterDesensitizer("carLicense", &DefaultDesensitizer{CarLicense})
	RegisterDesensitizer("bankCard", &DefaultDesensitizer{BankCard})
	RegisterDesensitizer("ipv4", &DefaultDesensitizer{IPV4})
	RegisterDesensitizer("ipv6", &DefaultDesensitizer{IPV6})
	RegisterDesensitizer("apiKey", &DefaultDesensitizer{APIKey})
	RegisterDesensitizer("apikey", &DefaultDesensitizer{APIKey})
	RegisterDesensitizer("secret", &DefaultDesensitizer{Secret})
	RegisterDesensitizer("secretKey", &DefaultDesensitizer{Secret})
	RegisterDesensitizer("userId", &DefaultDesensitizer{UserID})
	RegisterDesensitizer("userid", &DefaultDesensitizer{UserID})
	RegisterDesensitizer("playerId", &DefaultDesensitizer{PlayerID})
	RegisterDesensitizer("playerid", &DefaultDesensitizer{PlayerID})
	RegisterDesensitizer("orderNo", &DefaultDesensitizer{OrderNo})
	RegisterDesensitizer("orderno", &DefaultDesensitizer{OrderNo})
	RegisterDesensitizer("uuid", &DefaultDesensitizer{UUID})
	RegisterDesensitizer("openId", &DefaultDesensitizer{OpenID})
	RegisterDesensitizer("openid", &DefaultDesensitizer{OpenID})
	RegisterDesensitizer("unionId", &DefaultDesensitizer{OpenID})
	RegisterDesensitizer("account", &DefaultDesensitizer{Account})
}
