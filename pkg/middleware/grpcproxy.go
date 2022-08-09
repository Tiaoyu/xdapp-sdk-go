package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/hprose/hprose-golang/v3/rpc/core"
	"strings"
	"time"

	"github.com/Tiaoyu/xdapp-sdk-go/pkg/register"
	"github.com/fullstorydev/grpcurl"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/hprose/hprose-golang/v3/io"
	"github.com/hprose/hprose-golang/v3/rpc"
	"github.com/jhump/protoreflect/desc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	AdminIdHeaderKey   = "x-xdapp-admin-id"
	AppIdHeaderKey     = "x-xdapp-app-id"
	ServiceIdHeaderKey = "x-xdapp-service-id"
	RequestIdHeaderKey = "x-xdapp-request-id"
)

type GRPCProxyMiddleware struct {
	descSource    grpcurl.DescriptorSource
	nc            *grpc.ClientConn
	buf           *bytes.Buffer
	rParser       grpcurl.RequestParser
	resolver      jsonpb.AnyResolver
	respFormatter jsonpb.Marshaler
	lastResp      []byte

	Timeout time.Duration
	client  *core.Client
}

func New(endpoint string, descFileNames []string, opts ...grpc.DialOption) (*GRPCProxyMiddleware, error) {
	descSource, err := grpcurl.DescriptorSourceFromProtoSets(descFileNames...)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(nil)
	nc, err := grpc.Dial(endpoint, opts...)
	if err != nil {
		return nil, err
	}

	r := grpcurl.AnyResolverFromDescriptorSource(descSource)
	mid := &GRPCProxyMiddleware{
		descSource: descSource,
		nc:         nc,
		buf:        buf,
		resolver:   r,
		respFormatter: jsonpb.Marshaler{
			EnumsAsInts:  true,
			EmitDefaults: true,
			AnyResolver:  r,
		},
		client: core.NewClient(endpoint),
	}

	mid.regFunctions()
	return mid, nil
}

func (m *GRPCProxyMiddleware) IOHandler(ctx context.Context, request []byte, next core.NextIOHandler) (response []byte, err error) {
	method, params, err := m.parseInputData(request)
	if err != nil {
		return nil, err
	}

	if strings.HasPrefix(method, "sys_") {
		return next(ctx, request)
	}

	header := make([]string, 0)
	header = append(header, fmt.Sprintf("%s: %d", AdminIdHeaderKey, ctx.Value("adminId")))
	header = append(header, fmt.Sprintf("%s: %d", AppIdHeaderKey, ctx.Value("appId")))
	header = append(header, fmt.Sprintf("%s: %d", ServiceIdHeaderKey, ctx.Value("serviceId")))
	header = append(header, fmt.Sprintf("%s: %d", RequestIdHeaderKey, ctx.Value("requestId")))

	return m.requestProxy(context.Background(), m.parseGRPCMethod(method), params, header)
}

func (m *GRPCProxyMiddleware) InvokeHandler(ctx context.Context, name string, args []interface{}, next core.NextInvokeHandler) (result []interface{}, err error) {
	return nil, nil
}

func NewGRPCProxyMiddleware(endpoint string, descFileNames []string, opts ...grpc.DialOption) (*GRPCProxyMiddleware, error) {
	descSource, err := grpcurl.DescriptorSourceFromProtoSets(descFileNames...)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(nil)
	nc, err := grpc.Dial(endpoint, opts...)
	if err != nil {
		return nil, err
	}

	r := grpcurl.AnyResolverFromDescriptorSource(descSource)
	mid := &GRPCProxyMiddleware{
		descSource: descSource,
		nc:         nc,
		buf:        buf,
		resolver:   r,
		respFormatter: jsonpb.Marshaler{
			EnumsAsInts:  true,
			EmitDefaults: true,
			AnyResolver:  r,
		},
	}

	mid.regFunctions()
	return mid, nil
}

func (m *GRPCProxyMiddleware) regFunctions() {
	services, _ := m.descSource.ListServices()
	for _, srvName := range services {
		if m, err := m.descSource.FindSymbol(srvName); err == nil {
			if ms, ok := m.(*desc.ServiceDescriptor); ok {
				for _, sd := range ms.GetMethods() {
					funcName := srvName + "." + sd.GetName()
					register.AddFunction(funcName, func() {})
				}
			}
		}
	}
}

func (m *GRPCProxyMiddleware) OnResolveMethod(descriptor *desc.MethodDescriptor) {
}

func (m *GRPCProxyMiddleware) OnSendHeaders(md metadata.MD) {
}

func (m *GRPCProxyMiddleware) OnReceiveHeaders(md metadata.MD) {
}

func (m *GRPCProxyMiddleware) OnReceiveResponse(message proto.Message) {
	var buf bytes.Buffer
	err := m.respFormatter.Marshal(&buf, message)
	if err != nil {
		m.lastResp = m.parseRespErr(err)
		return
	}

	var response map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &response); err != nil {
		m.lastResp = m.parseRespErr(err)
		return
	}

	sb := &strings.Builder{}
	en := io.NewEncoder(sb)
	en.Simple(false)

	en.WriteTag(io.TagResult)
	en.Encode(response)
	en.WriteTag(io.TagEnd)
	m.lastResp = en.Bytes()
}

func (m *GRPCProxyMiddleware) parseRespErr(err error) []byte {
	sb := &strings.Builder{}
	en := io.NewEncoder(sb)
	en.Simple(false)
	en.WriteTag(io.TagError)
	en.Encode(err.Error())
	en.WriteTag(io.TagEnd)
	return en.Bytes()
}

func (m *GRPCProxyMiddleware) OnReceiveTrailers(status *status.Status, md metadata.MD) {
	if status.Code() != codes.OK {
		m.lastResp = m.parseRespErr(status.Err())
	}
}

func (m *GRPCProxyMiddleware) Handler(data []byte, ctx context.Context, next rpc.NextIOHandler) ([]byte, error) {
	method, params, err := m.parseInputData(data)
	if err != nil {
		return nil, err
	}

	if strings.HasPrefix(method, "sys_") {
		return next(ctx, data)
	}

	header := make([]string, 0)
	header = append(header, fmt.Sprintf("%s: %d", AdminIdHeaderKey, ctx.Value("adminId")))
	header = append(header, fmt.Sprintf("%s: %d", AppIdHeaderKey, ctx.Value("appId")))
	header = append(header, fmt.Sprintf("%s: %d", ServiceIdHeaderKey, ctx.Value("serviceId")))
	header = append(header, fmt.Sprintf("%s: %d", RequestIdHeaderKey, ctx.Value("requestId")))
	return m.requestProxy(context.Background(), m.parseGRPCMethod(method), params, header)
}

func (m *GRPCProxyMiddleware) requestProxy(context context.Context, methodName string, params []interface{}, header []string) ([]byte, error) {
	var data []byte
	var err error

	if len(params) > 0 {
		data, err = json.Marshal(params[0])
		if err != nil {
			println(err.Error())
			return nil, err
		}
	}

	m.buf.Write(data)
	defer m.buf.Reset()
	rf := grpcurl.NewJSONRequestParser(m.buf, m.resolver)
	err = grpcurl.InvokeRPC(context, m.descSource, m.nc, methodName, header, m, rf.Next)
	if err != nil {
		return nil, err
	}

	resp := m.lastResp
	m.lastResp = nil
	return resp, nil
}

func (m *GRPCProxyMiddleware) parseInputData(data []byte) (string, []interface{}, error) {
	var (
		tag    byte
		method string
		params []interface{}
	)

	reader := io.NewDecoder(data)
	reader.Simple(false)
	reader.MapType = io.MapTypeSIMap

	tag = reader.NextByte()
	if tag == io.TagCall {
		reader.Decode(&method)
		tag = reader.NextByte()
		if tag == io.TagList {
			reader.Reset()
			reader.Decode(&params, tag)
		}
	}

	return method, params, nil
}

func (m *GRPCProxyMiddleware) parseGRPCMethod(str string) string {
	service := strings.ReplaceAll(str[:strings.LastIndex(str, "_")], "_", ".")
	method := str[strings.LastIndex(str, "_")+1:]
	return service + "/" + method
}
