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
	"regexp"
	"strconv"
	"strings"
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

// DesensitizerFactory 参数化脱敏器工厂
// 根据标签中的整数参数创建脱敏器，如 jump(3,-4,1) 中的 3、-4、1
type DesensitizerFactory func(params ...int) Desensitizer

var desensitizerFactories map[string]DesensitizerFactory // 存储注册的参数化脱敏器工厂

// RegisterDesensitizerFactory 注册参数化脱敏器工厂
// @param name: 脱敏器名称（标签中括号前的名称，如 jump）
// @param factory: 根据参数创建脱敏器的工厂函数
func RegisterDesensitizerFactory(name string, factory DesensitizerFactory) {
	syncx.WithLock(&desensitizerMu, func() {
		if desensitizerFactories == nil {
			desensitizerFactories = make(map[string]DesensitizerFactory) // 初始化工厂映射
		}
		desensitizerFactories[name] = factory // 注册工厂
	})
}

// JumpDesensitizer 跳步脱敏器
// 按开始、结束、跳步参数对字符进行掩码，参数含义见 SensitizeJump
type JumpDesensitizer struct {
	start int // 开始索引（含），支持负数
	end   int // 结束索引（不含），支持负数（-N 表示保留末尾 N 个字符，0 表示到末尾）
	step  int // 跳步数
}

// Desensitize 方法实现 按跳步规则对输入的值进行脱敏处理
func (j *JumpDesensitizer) Desensitize(value string) string {
	return SensitizeJump(value, j.start, j.end, j.step)
}

// NewJumpDesensitizer 创建跳步脱敏器
// 参数依次为开始、结束、跳步，均可省略，省略时默认从头到尾连续掩码
func NewJumpDesensitizer(params ...int) Desensitizer {
	jumper := &JumpDesensitizer{step: 1}
	if len(params) > 0 {
		jumper.start = params[0]
	}
	if len(params) > 1 {
		jumper.end = params[1]
	}
	if len(params) > 2 {
		jumper.step = params[2]
	}
	return jumper
}

// tagRuleRegex 匹配带参数的脱敏标签，如 jump(3,-4,1)
var tagRuleRegex = regexp.MustCompile(`^([^\s(),]+)\s*\(([^)]*)\)$`)

// parseParameterizedTag 解析带参数的脱敏标签
// @param tag: 标签内容，如 jump(3,-4,1)
// @returns
//   - 规则名称、整数参数列表、是否为带参数形式
func parseParameterizedTag(tag string) (name string, params []int, ok bool) {
	matches := tagRuleRegex.FindStringSubmatch(tag)
	if matches == nil {
		return "", nil, false
	}
	name = matches[1]
	argStr := strings.TrimSpace(matches[2])
	if argStr == "" {
		return name, nil, true // 无参数形式，如 jump()
	}
	for _, part := range strings.Split(argStr, ",") {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return "", nil, false // 参数解析失败视为非法标签
		}
		params = append(params, value)
	}
	return name, params, true
}

// resolveDesensitizer 解析脱敏器（支持普通名称与带参数形式，需在持有 desensitizerMu 锁时调用）
// @param tag: 标签内容，如 email 或 jump(3,-4,1)
// @returns
//   - 返回脱敏器实例和可能的错误
func resolveDesensitizer(tag string) (Desensitizer, error) {
	// 带参数形式：通过工厂创建，如 jump(3,-4,1)
	if name, params, ok := parseParameterizedTag(tag); ok {
		if factory, exists := desensitizerFactories[name]; exists {
			return factory(params...), nil
		}
		return nil, errors.New("desensitizer factory not found")
	}
	// 普通形式：优先精确匹配已注册的脱敏器
	if desensitizer, exists := desensitizers[tag]; exists {
		return desensitizer, nil
	}
	// 普通形式未注册时，尝试以默认参数使用工厂，如 jump
	if factory, exists := desensitizerFactories[tag]; exists {
		return factory(), nil
	}
	return nil, errors.New("desensitizer not found")
}

// OperateByRule 根据规则进行脱敏操作
// @param desensitizerType: 脱敏器的类型标识，支持普通名称（如 email）和带参数形式（如 jump(3,-4,1)）。
// @param in: 需要脱敏的输入值，应该是字符串类型。
// @returns
//   - 返回脱敏后的值和可能的错误。
func OperateByRule(desensitizerType string, in interface{}) (interface{}, error) {
	return syncx.WithLockReturn(&desensitizerMu, func() (interface{}, error) {
		operator, err := resolveDesensitizer(desensitizerType) // 解析脱敏器
		if err != nil {
			return nil, err
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

	// 注册跳步脱敏器工厂：标签格式为 jump(开始,结束,跳步)，如 desensitize:"jump(3,-4,1)"
	RegisterDesensitizerFactory("jump", NewJumpDesensitizer)
}
