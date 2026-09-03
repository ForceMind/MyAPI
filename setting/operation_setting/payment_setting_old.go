/**
此文件为旧版支付设置文件，如需增加新的参数、变量等，请在 payment_setting.go 中添加
This file is the old version of the payment settings file. If you need to add new parameters, variables, etc., please add them in payment_setting.go
*/

package operation_setting

import (
	"sync"

	"github.com/ForceMind/MyAPI/common"
)

var PayAddress = ""
var CustomCallbackAddress = ""
var EpayId = ""
var EpayKey = ""
var Price = 7.3
var MinTopUp = 1
var USDExchangeRate = 7.3

var PayMethods = []map[string]string{
	{
		"name": "支付宝",
		"icon": "SiAlipay",
		"type": "alipay",
	},
	{
		"name": "微信",
		"icon": "SiWechat",
		"type": "wxpay",
	},
	{
		"name":      "自定义1",
		"icon":      "LuCreditCard",
		"type":      "custom1",
		"min_topup": "50",
	},
}
var payMethodsMutex sync.RWMutex

func UpdatePayMethodsByJsonString(jsonString string) error {
	var methods []map[string]string
	if err := common.Unmarshal([]byte(jsonString), &methods); err != nil {
		return err
	}
	if methods == nil {
		methods = []map[string]string{}
	}
	payMethodsMutex.Lock()
	PayMethods = methods
	payMethodsMutex.Unlock()
	return nil
}

func ValidatePayMethodsJSON(value string) error {
	var methods []map[string]string
	return common.Unmarshal([]byte(value), &methods)
}

func PayMethods2JsonString() string {
	payMethodsMutex.RLock()
	defer payMethodsMutex.RUnlock()
	jsonBytes, err := common.Marshal(PayMethods)
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

func GetPayMethods() []map[string]string {
	payMethodsMutex.RLock()
	defer payMethodsMutex.RUnlock()
	methods := make([]map[string]string, len(PayMethods))
	for i, method := range PayMethods {
		if method == nil {
			continue
		}
		methods[i] = make(map[string]string, len(method))
		for key, value := range method {
			methods[i][key] = value
		}
	}
	return methods
}

func ContainsPayMethod(method string) bool {
	payMethodsMutex.RLock()
	defer payMethodsMutex.RUnlock()
	for _, payMethod := range PayMethods {
		if payMethod["type"] == method {
			return true
		}
	}
	return false
}
