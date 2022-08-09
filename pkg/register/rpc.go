package register

import (
	"context"
	"errors"
	"fmt"
	"github.com/hprose/hprose-golang/v3/rpc"
	"github.com/hprose/hprose-golang/v3/rpc/core"
	"reflect"
	"strings"

	"github.com/hprose/hprose-golang/v3/io"
)

// 屏蔽列表输出
//func DoFunctionList() string {
//	return "Fa{}z"
//}

var hproseService *core.Service

// 执行结果
func (reg *register) RpcHandle(header Header, data []byte) ([]byte, error) {
	hproseContext := rpc.NewServiceContext(reg.HproseService)
	hproseContext.RequestHeaders().Set("appId", uint(header.AppId))
	hproseContext.RequestHeaders().Set("serviceId", uint(header.ServiceId))
	hproseContext.RequestHeaders().Set("requestId", uint(header.RequestId))
	hproseContext.RequestHeaders().Set("adminId", uint(header.AdminId))
	return reg.HproseService.Handle(core.WithContext(context.TODO(), hproseContext), data)
}

// 已注册的rpc方法
func (reg *register) GetHproseAddedFunc() []string {
	return reg.HproseService.Names()
}

// Simple 简单数据 https://github.com/hprose/hprose-golang/wiki/Hprose-%E6%9C%8D%E5%8A%A1%E5%99%A8
func (reg *register) AddFunction(name string, function interface{}) {
	reg.HproseService.AddFunction(function, name)
}

// 注册一个前端页面可访问的方法
func (reg *register) AddSysFunction(obj interface{}) {
	reg.HproseService.AddInstanceMethods(obj, "sys")
}

//// 增加过滤器 hprose v3 没有AddFilter
//func (reg *register) AddFilter(filter ...rpc.Filter) {
//	reg.HproseService.AddFilter(filter...)
//}

// 注册一个前端页面可访问的方法
func (reg *register) AddWebFunction(name string, function interface{}) {
	funcName := fmt.Sprintf("%s_%s", config.Name, name)
	reg.HproseService.AddFunction(function, funcName)
}

// Simple 简单数据 https://github.com/hprose/hprose-golang/wiki/Hprose-%E6%9C%8D%E5%8A%A1%E5%99%A8
func AddFunction(name string, function interface{}) {
	hproseService.AddFunction(function, name)
}

func (reg *register) AddWebInstanceMethods(obj interface{}, namespace string) {
	nsName := reg.cfg.Name
	if namespace != "" {
		nsName = fmt.Sprintf("%s_%s", reg.cfg.Name, namespace)
	}
	reg.HproseService.AddInstanceMethods(obj, nsName)
}

func (reg *register) AddBeforeFilterHandler(proxy func(ctx context.Context, request []byte, next core.NextIOHandler) (response []byte, err error)) {
	reg.HproseService.Use(proxy)
}

func rpcEncode(name string, args []reflect.Value) []byte {
	sb := &strings.Builder{}
	en := io.NewEncoder(sb)
	en.WriteTag(io.TagCall)
	en.WriteString(name)
	en.Reset()
	en.Encode(args)
	en.WriteTag(io.TagEnd)
	return en.Bytes()
}

func rpcDecode(data []byte) (interface{}, error) {
	r := io.GetDecoder().ResetBytes(data)
	r.Simple(false)
	r.MapType = io.MapTypeSIMap
	tag := r.NextByte()
	switch tag {
	case io.TagResult:
		var e interface{}
		r.Decode(&e)
		return e, nil
	case io.TagError:
		return nil, errors.New("RPC 系统调用 Agent 返回错误信息: " + r.ReadString())
	default:
		return nil, errors.New("RPC 系统调用收到一个未定义的方法返回: " + string(tag) + r.ReadString())
	}
}
