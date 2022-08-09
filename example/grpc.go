package main

import (
	"github.com/Tiaoyu/xdapp-sdk-go/pkg/middleware"
	"github.com/Tiaoyu/xdapp-sdk-go/pkg/register"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	reg, err := register.New(&register.Config{
		App:   "torchlight", // 请修改对应的App缩写
		Name:  "version",    // 请填入服务名，若协议Package为xdapp.api.v1则填入xdapp即可
		Key:   "MW9ooJ5b0J", // 从服务管理中添加服务后获取
		Debug: true,
	})

	if err != nil {
		panic(err)
	}

	// grpc service IP地址
	address := "localhost:8999"
	// grpc协议描述文件，参考：https://github.com/fullstorydev/grpcurl#protoset-files
	descriptor := []string{"./example/grpc/test.protoset"}
	proxy, err := middleware.New(address, descriptor, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}

	reg.AddBeforeFilterHandler(proxy.IOHandler)

	reg.ConnectToDev()
}
