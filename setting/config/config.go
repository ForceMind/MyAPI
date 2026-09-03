package config

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/ForceMind/MyAPI/common"
)

// ConfigManager 统一管理所有配置
type ConfigManager struct {
	configs        map[string]interface{}
	mutex          sync.RWMutex
	operationMutex sync.RWMutex
}

var GlobalConfig = NewConfigManager()

// MapConfig lets a module own its synchronization and validation instead of
// exposing mutable fields to the generic reflection-based loader/exporter.
type MapConfig interface {
	ExportConfigMap() (map[string]string, error)
	UpdateConfigMap(map[string]string) error
}

// ErrMapConfigValidationUnsupported reports that a MapConfig has no pure validation contract.
var ErrMapConfigValidationUnsupported = errors.New("MapConfig does not support side-effect-free validation")

func NewConfigManager() *ConfigManager {
	return &ConfigManager{
		configs: make(map[string]interface{}),
	}
}

// Register 注册一个配置模块
func (cm *ConfigManager) Register(name string, config interface{}) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	cm.configs[name] = config
}

// Get 获取指定配置模块
func (cm *ConfigManager) Get(name string) interface{} {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()
	return cm.configs[name]
}

func (cm *ConfigManager) snapshotConfigs() map[string]interface{} {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()
	configs := make(map[string]interface{}, len(cm.configs))
	for name, registered := range cm.configs {
		configs[name] = registered
	}
	return configs
}

// LoadFromDB 从数据库加载配置
func (cm *ConfigManager) LoadFromDB(options map[string]string) error {
	cm.operationMutex.Lock()
	defer cm.operationMutex.Unlock()
	for name, config := range cm.snapshotConfigs() {
		prefix := name + "."
		configMap := make(map[string]string)

		// 收集属于此配置的所有选项
		for key, value := range options {
			if strings.HasPrefix(key, prefix) {
				configKey := strings.TrimPrefix(key, prefix)
				configMap[configKey] = value
			}
		}

		// 如果找到配置项，则更新配置
		if len(configMap) > 0 {
			if err := updateConfigFromMap(config, configMap); err != nil {
				common.SysError("failed to update config " + name + ": " + err.Error())
				continue
			}
		}
	}

	return nil
}

// SaveToDB 将配置保存到数据库
func (cm *ConfigManager) SaveToDB(updateFunc func(key, value string) error) error {
	cm.operationMutex.RLock()
	defer cm.operationMutex.RUnlock()
	for name, config := range cm.snapshotConfigs() {
		configMap, err := configToMap(config)
		if err != nil {
			return err
		}

		for key, value := range configMap {
			dbKey := name + "." + key
			if err := updateFunc(dbKey, value); err != nil {
				return err
			}
		}
	}

	return nil
}

// 辅助函数：将配置对象转换为map
func configToMap(config interface{}) (map[string]string, error) {
	if managed, ok := config.(MapConfig); ok {
		return managed.ExportConfigMap()
	}
	result := make(map[string]string)

	val := reflect.ValueOf(config)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return nil, nil
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// 跳过未导出字段
		if !fieldType.IsExported() {
			continue
		}

		// 获取json标签作为键名
		key := fieldType.Tag.Get("json")
		if key == "" || key == "-" {
			key = fieldType.Name
		}

		// 处理不同类型的字段
		var strValue string
		switch field.Kind() {
		case reflect.String:
			strValue = field.String()
		case reflect.Bool:
			strValue = strconv.FormatBool(field.Bool())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			strValue = strconv.FormatInt(field.Int(), 10)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			strValue = strconv.FormatUint(field.Uint(), 10)
		case reflect.Float32, reflect.Float64:
			strValue = strconv.FormatFloat(field.Float(), 'f', -1, 64)
		case reflect.Ptr:
			// 处理指针类型：如果非 nil，序列化指向的值
			if !field.IsNil() {
				bytes, err := common.Marshal(field.Interface())
				if err != nil {
					return nil, err
				}
				strValue = string(bytes)
			} else {
				// nil 指针序列化为 "null"
				strValue = "null"
			}
		case reflect.Map, reflect.Slice, reflect.Struct:
			// 复杂类型使用JSON序列化
			bytes, err := common.Marshal(field.Interface())
			if err != nil {
				return nil, err
			}
			strValue = string(bytes)
		default:
			// 跳过不支持的类型
			continue
		}

		result[key] = strValue
	}

	return result, nil
}

// 辅助函数：从map更新配置对象
func updateConfigFromMap(config interface{}, configMap map[string]string) error {
	if managed, ok := config.(MapConfig); ok {
		return managed.UpdateConfigMap(configMap)
	}

	val, candidate, ok, err := prepareConfigFromMap(config, configMap)
	if err != nil {
		return err
	}
	if ok {
		val.Set(candidate)
	}
	return nil
}

func prepareConfigFromMap(config interface{}, configMap map[string]string) (reflect.Value, reflect.Value, bool, error) {
	val := reflect.ValueOf(config)
	if val.Kind() != reflect.Ptr {
		return reflect.Value{}, reflect.Value{}, false, nil
	}
	val = val.Elem()

	if val.Kind() != reflect.Struct {
		return reflect.Value{}, reflect.Value{}, false, nil
	}

	typ := val.Type()
	candidate := reflect.New(typ).Elem()
	candidate.Set(val)
	for i := 0; i < candidate.NumField(); i++ {
		field := candidate.Field(i)
		fieldType := typ.Field(i)

		// 跳过未导出字段
		if !fieldType.IsExported() {
			continue
		}

		// 获取json标签作为键名
		key := fieldType.Tag.Get("json")
		if key == "" || key == "-" {
			key = fieldType.Name
		}

		// 检查map中是否有对应的值
		strValue, ok := configMap[key]
		if !ok {
			continue
		}

		if !field.CanSet() {
			continue
		}

		parsed, err := parseConfigField(field, strValue)
		if err != nil {
			return reflect.Value{}, reflect.Value{}, false, fmt.Errorf("config field %q: %w", key, err)
		}
		field.Set(parsed)
	}

	return val, candidate, true, nil
}

func parseConfigField(field reflect.Value, strValue string) (reflect.Value, error) {
	parsed := reflect.New(field.Type()).Elem()

	switch field.Kind() {
	case reflect.String:
		parsed.SetString(strValue)
	case reflect.Bool:
		value, err := strconv.ParseBool(strValue)
		if err != nil {
			return reflect.Value{}, err
		}
		parsed.SetBool(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		bits := field.Type().Bits()
		value, err := strconv.ParseInt(strValue, 10, bits)
		if err != nil {
			floatValue, floatErr := strconv.ParseFloat(strValue, 64)
			minValue := -math.Ldexp(1, bits-1)
			maxValue := math.Ldexp(1, bits-1)
			if floatErr != nil || math.IsNaN(floatValue) || math.IsInf(floatValue, 0) || math.Trunc(floatValue) != floatValue || floatValue < minValue || floatValue >= maxValue {
				return reflect.Value{}, err
			}
			value = int64(floatValue)
		}
		parsed.SetInt(value)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		bits := field.Type().Bits()
		value, err := strconv.ParseUint(strValue, 10, bits)
		if err != nil {
			floatValue, floatErr := strconv.ParseFloat(strValue, 64)
			maxValue := math.Ldexp(1, bits)
			if floatErr != nil || math.IsNaN(floatValue) || math.IsInf(floatValue, 0) || math.Trunc(floatValue) != floatValue || floatValue < 0 || floatValue >= maxValue {
				return reflect.Value{}, err
			}
			value = uint64(floatValue)
		}
		parsed.SetUint(value)
	case reflect.Float32, reflect.Float64:
		value, err := strconv.ParseFloat(strValue, field.Type().Bits())
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			if err == nil {
				err = fmt.Errorf("value must be finite")
			}
			return reflect.Value{}, err
		}
		parsed.SetFloat(value)
	case reflect.Ptr:
		parsed.Set(cloneConfigValue(field))
		if strValue == "null" {
			parsed.Set(reflect.Zero(field.Type()))
		} else {
			if parsed.IsNil() {
				parsed.Set(reflect.New(field.Type().Elem()))
			}
			if err := common.Unmarshal([]byte(strValue), parsed.Interface()); err != nil {
				return reflect.Value{}, err
			}
		}
	case reflect.Map:
		fresh := reflect.New(field.Type())
		if err := common.Unmarshal([]byte(strValue), fresh.Interface()); err != nil {
			return reflect.Value{}, err
		}
		parsed.Set(fresh.Elem())
	case reflect.Slice, reflect.Struct:
		parsed.Set(cloneConfigValue(field))
		if err := common.Unmarshal([]byte(strValue), parsed.Addr().Interface()); err != nil {
			return reflect.Value{}, err
		}
	default:
		parsed.Set(field)
	}

	return parsed, nil
}

func cloneConfigValue(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}

	switch value.Kind() {
	case reflect.Ptr:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		clone := reflect.New(value.Type().Elem())
		clone.Elem().Set(cloneConfigValue(value.Elem()))
		return clone
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		clone := reflect.New(value.Type()).Elem()
		clone.Set(cloneConfigValue(value.Elem()))
		return clone
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		clone := reflect.MakeMapWithSize(value.Type(), value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			clone.SetMapIndex(cloneConfigValue(iterator.Key()), cloneConfigValue(iterator.Value()))
		}
		return clone
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		clone := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := 0; i < value.Len(); i++ {
			clone.Index(i).Set(cloneConfigValue(value.Index(i)))
		}
		return clone
	case reflect.Array:
		clone := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			clone.Index(i).Set(cloneConfigValue(value.Index(i)))
		}
		return clone
	case reflect.Struct:
		clone := reflect.New(value.Type()).Elem()
		clone.Set(value)
		for i := 0; i < value.NumField(); i++ {
			if value.Type().Field(i).IsExported() {
				clone.Field(i).Set(cloneConfigValue(value.Field(i)))
			}
		}
		return clone
	default:
		return value
	}
}

// ConfigToMap 将配置对象转换为map（导出函数）
func ConfigToMap(config interface{}) (map[string]string, error) {
	return configToMap(config)
}

// UpdateConfigFromMap 从map更新配置对象（导出函数）
func UpdateConfigFromMap(config interface{}, configMap map[string]string) error {
	return updateConfigFromMap(config, configMap)
}

// ValidateConfigFromMap validates an update without changing config.
func ValidateConfigFromMap(config interface{}, configMap map[string]string) error {
	if _, ok := config.(MapConfig); ok {
		return ErrMapConfigValidationUnsupported
	}
	_, _, _, err := prepareConfigFromMap(config, configMap)
	return err
}

// ExportAllConfigs 导出所有已注册的配置为扁平结构
func (cm *ConfigManager) ExportAllConfigs() map[string]string {
	cm.operationMutex.RLock()
	defer cm.operationMutex.RUnlock()
	result := make(map[string]string)

	for name, cfg := range cm.snapshotConfigs() {
		configMap, err := ConfigToMap(cfg)
		if err != nil {
			continue
		}

		// 使用 "模块名.配置项" 的格式添加到结果中
		for key, value := range configMap {
			result[name+"."+key] = value
		}
	}

	return result
}
