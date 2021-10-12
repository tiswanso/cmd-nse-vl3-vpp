package vl3_nse

import (
	"context"
	"fmt"
	"github.com/golang/protobuf/ptypes/empty"

	"github.com/networkservicemesh/api/pkg/api/networkservice"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/next"
	"net"
)

type vl3Server struct {
	//Map
	routeCidrs []*net.IPNet
}

// NewServer - creates a new NetworkServiceServer chain element that implements IPAM service.
func NewServer(srcRouteCidrs ...*net.IPNet) networkservice.NetworkServiceServer {
	return &vl3Server{
		routeCidrs: srcRouteCidrs,
	}
}

func (s *vl3Server) Request(ctx context.Context, request *networkservice.NetworkServiceRequest) (*networkservice.Connection, error) {

	conn := request.GetConnection()
	if conn.GetContext() == nil {
		conn.Context = &networkservice.ConnectionContext{}
	}
	if conn.GetContext().GetIpContext() == nil {
		//conn.GetContext().IpContext = &networkservice.IPContext{}
		return nil, fmt.Errorf("vl3Server requires IPAM chain element in network service endpoint")
	}
	ipContext := conn.GetContext().GetIpContext()

	for _, routeCidr := range s.routeCidrs {
		for _, dstIp := range ipContext.DstIpAddrs {
			addRoute(&ipContext.SrcRoutes, routeCidr, dstIp)
		}
	}
	conn, err := next.Server(ctx).Request(ctx, request)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (s *vl3Server) Close(ctx context.Context, conn *networkservice.Connection) (_ *empty.Empty, err error) {
	return next.Server(ctx).Close(ctx, conn)
}

func addRoute(routes *[]*networkservice.Route, routeCidr *net.IPNet, nextHop string) {
	for _, route := range *routes {
		if route.Prefix == routeCidr.String() {
			return
		}
	}
	*routes = append(*routes, &networkservice.Route{
		Prefix: routeCidr.String(),
		NextHop: nextHop,
	})
}

